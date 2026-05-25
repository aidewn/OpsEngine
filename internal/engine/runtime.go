// 单次执行的运行时状态容器
// 所有节点状态/日志/变量都内化到 Frame 上，Runtime 持有根帧
// 每次集合调用创建子 Frame 并挂在父 Frame.Children[callerInstanceID]
// 并发分支 / 多 update tick 共享同一 Frame（通过 mu 串行化）

package engine

import (
	"context"
	"strconv"
	"sync"
	"time"

	"OpsEngine/internal/core"

	"github.com/google/uuid"
)

// Frame 调用栈帧（树节点）
// 主流 frame 是 Runtime.rootFrame，AssembleID = ""
type Frame struct {
	AssembleID string
	Variables  map[string]any
	Params     map[string]any
	Returns    map[string]any
	Parent     *Frame

	// 该 frame 内的执行数据
	NodeStates map[string]core.NodeState
	NodeLogs   map[string][]core.LogEntry
	Outputs    map[string]Outputs

	// 子 frame：key = caller node instance ID
	Children map[string]*Frame

	// 路径（从根到该 frame 的 caller instance ID 序列）
	// 用于事件 payload 的 framePath
	Path []string
}

// Runtime 单次执行的状态容器
type Runtime struct {
	ID         string
	WorkflowID string
	Snapshot   core.ExecutionSnapshot
	StartedAt  time.Time

	rootFrame *Frame // 主流 frame（栈底，所有派生帧的根）

	// 以下字段需要 mu 保护
	status     core.WorkflowStatus
	finishedAt *time.Time
	errMsg     string

	// 追踪所有活跃 goroutine（含 thread spawn 的后台流）
	wg sync.WaitGroup

	ctx     context.Context
	cancel  context.CancelFunc
	emitter Emitter

	mu sync.Mutex
}

// newRuntime 创建运行时实例
func newRuntime(wf core.WorkflowDef, snapshot core.ExecutionSnapshot, emitter Emitter) *Runtime {
	ctx, cancel := context.WithCancel(context.Background())
	rt := &Runtime{
		ID:         uuid.New().String(),
		WorkflowID: wf.ID,
		Snapshot:   snapshot,
		StartedAt:  time.Now(),
		status:     core.WorkflowStatusRunning,
		ctx:        ctx,
		cancel:     cancel,
		emitter:    emitter,
	}
	rt.rootFrame = newFrame("", initVariables(wf.Variables), nil, nil)
	return rt
}

// newFrame 创建一个新的 Frame
func newFrame(assembleID string, vars map[string]any, parent *Frame, callerID *string) *Frame {
	path := []string{}
	if parent != nil {
		path = append(path, parent.Path...)
	}
	if callerID != nil {
		path = append(path, *callerID)
	}
	if vars == nil {
		vars = map[string]any{}
	}
	return &Frame{
		AssembleID: assembleID,
		Variables:  vars,
		Parent:     parent,
		NodeStates: map[string]core.NodeState{},
		NodeLogs:   map[string][]core.LogEntry{},
		Outputs:    map[string]Outputs{},
		Children:   map[string]*Frame{},
		Path:       path,
	}
}

// pushChildFrame 在 parent 上挂载子 frame（key 为 caller 节点 instance ID）
// 同一 caller 重复调用（如 update 多次 tick）会覆盖之前的 frame
func (r *Runtime) pushChildFrame(parent *Frame, callerInstanceID, assembleID string, vars, params map[string]any) *Frame {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := callerInstanceID
	f := newFrame(assembleID, vars, parent, &id)
	f.Params = params
	f.Returns = map[string]any{}
	parent.Children[callerInstanceID] = f
	return f
}

// ── 变量读写（在指定 frame 作用域） ────────────────────────

func (r *Runtime) getVariable(frame *Frame, name string) (any, bool) {
	if frame == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := frame.Variables[name]
	return v, ok
}

func (r *Runtime) setVariable(frame *Frame, name string, value any) {
	if frame == nil {
		return
	}
	r.mu.Lock()
	frame.Variables[name] = value
	r.mu.Unlock()

	r.emitter.Emit(EventVariable, map[string]any{
		"executionID": r.ID,
		"framePath":   frame.Path,
		"name":        name,
		"value":       value,
	})
}

// getParam 读 frame 的参数
func (r *Runtime) getParam(frame *Frame, name string) (any, bool) {
	if frame == nil || frame.Params == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := frame.Params[name]
	return v, ok
}

// setReturn 写 frame 的返回值（assemble frame 内由 return_set 节点调用）
func (r *Runtime) setReturn(frame *Frame, name string, value any) {
	if frame == nil || frame.Returns == nil {
		return
	}
	r.mu.Lock()
	frame.Returns[name] = value
	r.mu.Unlock()
}

// ── 节点状态推送（按 frame） ──────────────────────────────

func (r *Runtime) setNodeState(frame *Frame, nodeID string, state core.NodeState, errMsg string) {
	r.mu.Lock()
	frame.NodeStates[nodeID] = state
	r.mu.Unlock()

	payload := map[string]any{
		"executionID": r.ID,
		"framePath":   frame.Path,
		"nodeID":      nodeID,
		"state":       state,
	}
	if errMsg != "" {
		payload["errorMsg"] = errMsg
	}
	r.emitter.Emit(EventNode, payload)
}

// ── 日志推送 ──────────────────────────────────────────────

func (r *Runtime) appendLog(frame *Frame, nodeID, level, msg string) {
	entry := core.LogEntry{Time: time.Now(), Level: level, Message: msg}
	r.mu.Lock()
	frame.NodeLogs[nodeID] = append(frame.NodeLogs[nodeID], entry)
	r.mu.Unlock()

	r.emitter.Emit(EventLog, map[string]any{
		"executionID": r.ID,
		"framePath":   frame.Path,
		"nodeID":      nodeID,
		"time":        entry.Time,
		"level":       level,
		"message":     msg,
	})
}

// ── 终态收尾 ──────────────────────────────────────────────

// markRemainingTerminated 递归把所有 frame 中仍在 Executing 的节点标记 Terminated
func (r *Runtime) markRemainingTerminated() {
	r.markFrameTerminated(r.rootFrame)
}

func (r *Runtime) markFrameTerminated(frame *Frame) {
	r.mu.Lock()
	var changed []string
	for nodeID, state := range frame.NodeStates {
		if state == core.NodeStateExecuting {
			frame.NodeStates[nodeID] = core.NodeStateTerminated
			changed = append(changed, nodeID)
		}
	}
	children := make([]*Frame, 0, len(frame.Children))
	for _, c := range frame.Children {
		children = append(children, c)
	}
	r.mu.Unlock()

	for _, nodeID := range changed {
		r.emitter.Emit(EventNode, map[string]any{
			"executionID": r.ID,
			"framePath":   frame.Path,
			"nodeID":      nodeID,
			"state":       core.NodeStateTerminated,
		})
	}
	for _, c := range children {
		r.markFrameTerminated(c)
	}
}

// ── 终态标记 ──────────────────────────────────────────────

func (r *Runtime) markSuccess()        { r.markFinished(core.WorkflowStatusSuccess, "") }
func (r *Runtime) markFailed(e string) { r.markFinished(core.WorkflowStatusFailed, e) }
func (r *Runtime) markTerminated()     { r.markFinished(core.WorkflowStatusTerminated, "") }

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
	return core.ExecutionRecord{
		ID:         r.ID,
		WorkflowID: r.WorkflowID,
		Snapshot:   r.Snapshot,
		Status:     r.status,
		StartedAt:  r.StartedAt,
		FinishedAt: r.finishedAt,
		RootFrame:  serializeFrame(r.rootFrame),
		Error:      r.errMsg,
	}
}

// serializeFrame 递归把 Frame 转成 core.FrameState（深拷贝）
func serializeFrame(f *Frame) core.FrameState {
	state := core.FrameState{
		AssembleID: f.AssembleID,
		NodeStates: copyStateMap(f.NodeStates),
		NodeLogs:   copyLogsMap(f.NodeLogs),
		Variables:  copyAnyMap(f.Variables),
		Params:     copyAnyMap(f.Params),
		Returns:    copyAnyMap(f.Returns),
	}
	if len(f.Children) > 0 {
		state.Children = make(map[string]*core.FrameState, len(f.Children))
		for k, child := range f.Children {
			sub := serializeFrame(child)
			state.Children[k] = &sub
		}
	}
	return state
}

func copyStateMap(m map[string]core.NodeState) map[string]core.NodeState {
	if m == nil {
		return nil
	}
	out := make(map[string]core.NodeState, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
func copyLogsMap(m map[string][]core.LogEntry) map[string][]core.LogEntry {
	if m == nil {
		return nil
	}
	out := make(map[string][]core.LogEntry, len(m))
	for k, v := range m {
		cp := make([]core.LogEntry, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}
func copyAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
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

func initVariables(defs []core.VariableDef) map[string]any {
	vars := map[string]any{}
	for _, d := range defs {
		vars[d.Name] = coerceDefault(d.Default, d.VarType)
	}
	return vars
}

func coerceDefault(raw any, varType core.PortType) any {
	if raw == nil {
		return zeroValue(varType)
	}
	s, ok := raw.(string)
	if !ok {
		return raw
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
		return nil
	}
}

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

// findNode 在节点列表中查找
func findNode(nodes []core.NodeInstance, id string) *core.NodeInstance {
	for i := range nodes {
		if nodes[i].InstanceID == id {
			return &nodes[i]
		}
	}
	return nil
}

// errMissingNode 节点 ID 不存在时的错误
func errMissingNode(id string) error {
	return errMissingNodeErr{id: id}
}

type errMissingNodeErr struct{ id string }

func (e errMissingNodeErr) Error() string { return "节点 " + e.id + " 不存在于快照中" }
