// 单次执行的运行时状态容器
// 维护：节点状态 / 节点日志 / 节点输出缓存 / 变量帧栈 / 取消信号

package engine

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"OpsEngine/internal/core"

	"github.com/google/uuid"
)

// Frame 调用栈帧
// 主工作流是 frame 0，进入集合调用 push 新 frame，集合返回 pop
// MVP 阶段集合调用未实装，frame 栈始终只有主帧
type Frame struct {
	AssembleID string         // 帧对应的集合 ID；主帧为空
	Variables  map[string]any // 本帧的变量作用域
	Params     map[string]any // 调用方传入的参数（仅 assemble 帧）
	Returns    map[string]any // 待返回给调用者的值
	Parent     *Frame
}

// Runtime 单次执行的状态容器
// 字段都是私有，外部通过 Record() / Summary() / 事件流读取
//
// 并发模型：
//   - frames 不再是 Runtime 的全局字段，由每个执行流（goroutine）通过 FrameStack 维护
//   - mainFrame 是栈底根帧，所有栈共享同一个对象
//   - Frame 内部的 map 读写通过 Runtime.mu 串行化
type Runtime struct {
	ID         string
	WorkflowID string
	Snapshot   core.ExecutionSnapshot
	StartedAt  time.Time

	// 主帧（栈底根帧），保存工作流变量
	// 多个执行流共享该对象，所有变量修改通过 mu 保护
	mainFrame *Frame

	// 以下字段需要 mu 保护
	status     core.WorkflowStatus
	finishedAt *time.Time
	errMsg     string
	nodeStates map[string]core.NodeState
	nodeLogs   map[string][]core.LogEntry
	outputs    map[string]Outputs // 已执行 action 节点的输出缓存

	// 追踪所有活跃 goroutine（含 thread spawn 的后台流）
	// runMain 等待 wg 归零再标记终态
	wg sync.WaitGroup

	ctx     context.Context
	cancel  context.CancelFunc
	emitter Emitter

	mu sync.Mutex
}

// newRuntime 创建运行时实例
// 主帧根据 workflow.Variables 的 default 初始化
func newRuntime(wf core.WorkflowDef, snapshot core.ExecutionSnapshot, emitter Emitter) *Runtime {
	ctx, cancel := context.WithCancel(context.Background())
	rt := &Runtime{
		ID:         uuid.New().String(),
		WorkflowID: wf.ID,
		Snapshot:   snapshot,
		StartedAt:  time.Now(),
		status:     core.WorkflowStatusRunning,
		nodeStates: map[string]core.NodeState{},
		nodeLogs:   map[string][]core.LogEntry{},
		outputs:    map[string]Outputs{},
		ctx:        ctx,
		cancel:     cancel,
		emitter:    emitter,
	}
	rt.mainFrame = &Frame{Variables: initVariables(wf.Variables)}
	return rt
}

// newMainStack 创建一个以主帧为底的新栈
// 每次启动主流 / 主线程时调用
func (r *Runtime) newMainStack() *FrameStack {
	return newStack(r.mainFrame)
}

// ── 变量读写（在指定栈的当前帧作用域） ────────────────────

func (r *Runtime) getVariable(stack *FrameStack, name string) (any, bool) {
	f := stack.current()
	if f == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := f.Variables[name]
	return v, ok
}

func (r *Runtime) setVariable(stack *FrameStack, name string, value any) {
	f := stack.current()
	if f == nil {
		return
	}
	r.mu.Lock()
	f.Variables[name] = value
	r.mu.Unlock()

	r.emitter.Emit(EventVariable, map[string]any{
		"executionID": r.ID,
		"name":        name,
		"value":       value,
	})
}

// getParam 读栈顶帧的参数
func (r *Runtime) getParam(stack *FrameStack, name string) (any, bool) {
	f := stack.current()
	if f == nil || f.Params == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := f.Params[name]
	return v, ok
}

// ── 节点状态推送 ──────────────────────────────────────────

func (r *Runtime) setNodeState(nodeID string, state core.NodeState, errMsg string) {
	r.mu.Lock()
	r.nodeStates[nodeID] = state
	r.mu.Unlock()

	payload := map[string]any{
		"executionID": r.ID,
		"nodeID":      nodeID,
		"state":       state,
	}
	if errMsg != "" {
		payload["errorMsg"] = errMsg
	}
	r.emitter.Emit(EventNode, payload)
}

// ── 日志推送 ──────────────────────────────────────────────

func (r *Runtime) appendLog(nodeID, level, msg string) {
	entry := core.LogEntry{Time: time.Now(), Level: level, Message: msg}
	r.mu.Lock()
	r.nodeLogs[nodeID] = append(r.nodeLogs[nodeID], entry)
	r.mu.Unlock()

	r.emitter.Emit(EventLog, map[string]any{
		"executionID": r.ID,
		"nodeID":      nodeID,
		"time":        entry.Time,
		"level":       level,
		"message":     msg,
	})
}

// ── 终态收尾 ──────────────────────────────────────────────

// markRemainingTerminated 把仍处于 Executing 的节点标记为 Terminated
// 用于 break / Stop 中断时把"被打断"的节点状态从 Executing → Terminated
// 推送对应事件让前端 UI 同步
func (r *Runtime) markRemainingTerminated() {
	r.mu.Lock()
	var changed []string
	for nodeID, state := range r.nodeStates {
		if state == core.NodeStateExecuting {
			r.nodeStates[nodeID] = core.NodeStateTerminated
			changed = append(changed, nodeID)
		}
	}
	r.mu.Unlock()

	for _, nodeID := range changed {
		r.emitter.Emit(EventNode, map[string]any{
			"executionID": r.ID,
			"nodeID":      nodeID,
			"state":       core.NodeStateTerminated,
		})
	}
}

// ── 终态标记 ──────────────────────────────────────────────

func (r *Runtime) markSuccess() {
	r.markFinished(core.WorkflowStatusSuccess, "")
}

func (r *Runtime) markFailed(errMsg string) {
	r.markFinished(core.WorkflowStatusFailed, errMsg)
}

func (r *Runtime) markTerminated() {
	r.markFinished(core.WorkflowStatusTerminated, "")
}

func (r *Runtime) markFinished(status core.WorkflowStatus, errMsg string) {
	t := time.Now()
	r.mu.Lock()
	r.status = status
	r.finishedAt = &t
	r.errMsg = errMsg
	r.mu.Unlock()

	payload := map[string]any{
		"executionID": r.ID,
		"status":      status,
	}
	if errMsg != "" {
		payload["error"] = errMsg
	}
	r.emitter.Emit(EventFinished, payload)
}

// ── 对外只读视图 ─────────────────────────────────────────

// Status 当前状态
func (r *Runtime) Status() core.WorkflowStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

// Record 返回完整执行记录的副本（用于 Wails 绑定返回前端）
func (r *Runtime) Record() core.ExecutionRecord {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 复制 map 防止外部并发改动
	nodeStates := make(map[string]core.NodeState, len(r.nodeStates))
	for k, v := range r.nodeStates {
		nodeStates[k] = v
	}
	nodeLogs := make(map[string][]core.LogEntry, len(r.nodeLogs))
	for k, v := range r.nodeLogs {
		logs := make([]core.LogEntry, len(v))
		copy(logs, v)
		nodeLogs[k] = logs
	}
	var vars map[string]any
	if r.mainFrame != nil {
		vars = make(map[string]any, len(r.mainFrame.Variables))
		for k, v := range r.mainFrame.Variables {
			vars[k] = v
		}
	}

	return core.ExecutionRecord{
		ID:         r.ID,
		WorkflowID: r.WorkflowID,
		Snapshot:   r.Snapshot,
		Status:     r.status,
		StartedAt:  r.StartedAt,
		FinishedAt: r.finishedAt,
		NodeStates: nodeStates,
		NodeLogs:   nodeLogs,
		Variables:  vars,
		Error:      r.errMsg,
	}
}

// Summary 列表用精简结构
func (r *Runtime) Summary() core.ExecutionSummary {
	r.mu.Lock()
	defer r.mu.Unlock()
	return core.ExecutionSummary{
		ID:           r.ID,
		WorkflowID:   r.WorkflowID,
		WorkflowName: r.Snapshot.Workflow.Name,
		Status:       r.status,
		StartedAt:    r.StartedAt,
		FinishedAt:   r.finishedAt,
		Error:        r.errMsg,
	}
}

// ── 变量默认值转换 ────────────────────────────────────────

// initVariables 根据 VariableDef 列表初始化变量 map
// default 字段按 var_type 转成对应 Go 类型
func initVariables(defs []core.VariableDef) map[string]any {
	vars := map[string]any{}
	for _, d := range defs {
		vars[d.Name] = coerceDefault(d.Default, d.VarType)
	}
	return vars
}

// coerceDefault 把 default 原始值（通常是 string）按变量类型转换
func coerceDefault(raw any, varType core.PortType) any {
	if raw == nil {
		return zeroValue(varType)
	}
	s, ok := raw.(string)
	if !ok {
		return raw // 已是具体类型
	}
	if s == "" {
		return zeroValue(varType)
	}
	switch varType {
	case core.PortTypeString:
		return s
	case core.PortTypeInt:
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return int64(0)
		}
		return v
	case core.PortTypeFloat:
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0.0
		}
		return v
	case core.PortTypeBool:
		return s == "true" || s == "1"
	default:
		return nil // 句柄类型无法从字面字符串构造
	}
}

// zeroValue 返回某类型的零值
func zeroValue(varType core.PortType) any {
	switch varType {
	case core.PortTypeString:
		return ""
	case core.PortTypeInt:
		return int64(0)
	case core.PortTypeFloat:
		return 0.0
	case core.PortTypeBool:
		return false
	default:
		return nil
	}
}

// ── 辅助：在节点列表里查找 ──────────────────────────────

func findNode(nodes []core.NodeInstance, id string) *core.NodeInstance {
	for i := range nodes {
		if nodes[i].InstanceID == id {
			return &nodes[i]
		}
	}
	return nil
}

// errMissingNode 节点 ID 不存在时的错误（提供给调用方便利使用）
func errMissingNode(id string) error {
	return fmt.Errorf("节点 %s 不存在于快照中", id)
}
