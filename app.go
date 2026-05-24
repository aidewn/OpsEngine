// 桌面应用主结构，所有 public 方法自动绑定到前端 JS

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	_ "OpsEngine/internal/nodes" // 触发所有内置节点的 init() 注册
	"OpsEngine/internal/store"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// 集合节点类型 ID 前缀，用于区分内置节点和集合调用节点
const assembleTypePrefix = "assemble:"

// App 桌面应用实例，public 方法通过 Wails 绑定暴露给前端调用
type App struct {
	ctx            context.Context
	workflowStore  *store.WorkflowStore
	assembleStore  *store.AssembleStore
	executionStore *store.ExecutionStore
	engine         *engine.Engine
}

// NewApp 创建应用实例
func NewApp() *App {
	return &App{}
}

// startup Wails 生命周期钩子，窗口创建前调用
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 初始化日志
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)

	// 确保数据目录存在
	for _, dir := range []string{"data/workflows", "data/assembles", "data/executions", "data/logs"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			zap.L().Fatal("创建目录失败", zap.Error(err))
		}
	}

	a.workflowStore = store.NewWorkflowStore("data/workflows")
	a.assembleStore = store.NewAssembleStore("data/assembles")
	a.executionStore = store.NewExecutionStore("data/executions")
	a.engine = engine.New(
		a.workflowStore,
		a.assembleStore,
		a.executionStore,
		engine.NewWailsEmitter(ctx),
	)
	zap.L().Info("OpsEngine 桌面应用启动完成")
}

// ── 工作流 CRUD ──────────────────────────────────────────────

// ListWorkflows 获取所有工作流
func (a *App) ListWorkflows() ([]core.WorkflowDef, error) {
	return a.workflowStore.List()
}

// GetWorkflow 按 ID 获取工作流详情
func (a *App) GetWorkflow(id string) (core.WorkflowDef, error) {
	return a.workflowStore.Get(id)
}

// CreateWorkflow 创建新工作流，返回生成的 ID
func (a *App) CreateWorkflow(name string, description string) (string, error) {
	wf := core.WorkflowDef{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		Nodes:       []core.NodeInstance{},
		Edges:       []core.EdgeConfig{},
	}
	if err := a.workflowStore.Save(wf); err != nil {
		return "", err
	}
	return wf.ID, nil
}

// UpdateWorkflow 整体覆盖更新工作流
// 保存前校验：单例节点 + exec 输出端口单连接
func (a *App) UpdateWorkflow(wf core.WorkflowDef) error {
	if err := engine.ValidateWorkflow(wf); err != nil {
		return err
	}
	return a.workflowStore.Save(wf)
}

// DeleteWorkflow 删除工作流
func (a *App) DeleteWorkflow(id string) error {
	return a.workflowStore.Delete(id)
}

// ── 集合 CRUD ───────────────────────────────────────────────

// ListAssembles 获取所有集合
func (a *App) ListAssembles() ([]core.AssembleDef, error) {
	return a.assembleStore.List()
}

// GetAssemble 按 ID 获取集合详情
func (a *App) GetAssemble(id string) (core.AssembleDef, error) {
	return a.assembleStore.Get(id)
}

// CreateAssemble 创建新集合，返回生成的 ID
func (a *App) CreateAssemble(name string, description string) (string, error) {
	a2 := core.AssembleDef{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		Params:      []core.ParamDef{},
		Returns:     []core.ParamDef{},
		Variables:   []core.VariableDef{},
		Nodes:       []core.NodeInstance{},
		Edges:       []core.EdgeConfig{},
	}
	if err := a.assembleStore.Save(a2); err != nil {
		return "", err
	}
	return a2.ID, nil
}

// UpdateAssemble 整体覆盖更新集合
// 保存前校验：结构合法性（单例 + exec_out 单连接） + 循环引用
func (a *App) UpdateAssemble(a2 core.AssembleDef) error {
	if err := engine.ValidateAssemble(a2); err != nil {
		return err
	}
	if err := a.checkCircularRef(a2); err != nil {
		return err
	}
	return a.assembleStore.Save(a2)
}

// checkCircularRef 检查集合 target 是否包含直接或间接引用自身的循环
func (a *App) checkCircularRef(target core.AssembleDef) error {
	visited := map[string]bool{}
	for _, refID := range extractAssembleRefs(target.Nodes) {
		if err := a.dfsCheckRef(target.ID, refID, visited); err != nil {
			return err
		}
	}
	return nil
}

// dfsCheckRef DFS 检测集合引用链中是否再次出现 targetID
func (a *App) dfsCheckRef(targetID, currentID string, visited map[string]bool) error {
	if targetID == currentID {
		return fmt.Errorf("循环引用：集合不能直接或间接引用自身")
	}
	if visited[currentID] {
		return nil
	}
	visited[currentID] = true

	sub, err := a.assembleStore.Get(currentID)
	if err != nil {
		// 引用的集合不存在，跳过（保留容错）
		return nil
	}
	for _, refID := range extractAssembleRefs(sub.Nodes) {
		if err := a.dfsCheckRef(targetID, refID, visited); err != nil {
			return err
		}
	}
	return nil
}

// extractAssembleRefs 从节点列表中提取所有被引用的集合 ID
func extractAssembleRefs(nodes []core.NodeInstance) []string {
	var refs []string
	for _, n := range nodes {
		if strings.HasPrefix(n.TypeID, assembleTypePrefix) {
			refs = append(refs, strings.TrimPrefix(n.TypeID, assembleTypePrefix))
		}
	}
	return refs
}

// DeleteAssemble 删除集合
func (a *App) DeleteAssemble(id string) error {
	return a.assembleStore.Delete(id)
}

// ── 执行 ─────────────────────────────────────────────────

// RunWorkflow 启动一次工作流执行
// 立即返回执行 ID；执行在后台 goroutine 中进行
// 前端通过 Wails 事件订阅状态/日志变化
func (a *App) RunWorkflow(workflowID string) (string, error) {
	return a.engine.Run(workflowID)
}

// StopWorkflow 取消执行
func (a *App) StopWorkflow(executionID string) error {
	return a.engine.Stop(executionID)
}

// ListExecutions 列出所有执行（内存 + 持久化，按开始时间倒序）
func (a *App) ListExecutions() []core.ExecutionSummary {
	return a.engine.ListSummaries()
}

// ListExecutionsByWorkflow 列出某工作流的所有执行（含历史）
func (a *App) ListExecutionsByWorkflow(workflowID string) []core.ExecutionSummary {
	return a.engine.ListSummariesByWorkflow(workflowID)
}

// GetExecution 获取执行详情（内存优先，找不到回持久化）
func (a *App) GetExecution(executionID string) (core.ExecutionRecord, error) {
	rec, ok := a.engine.GetRecord(executionID)
	if !ok {
		return core.ExecutionRecord{}, fmt.Errorf("执行 %s 不存在", executionID)
	}
	return rec, nil
}

// DeleteExecution 从内存移除执行记录（仅终态可删）
func (a *App) DeleteExecution(executionID string) error {
	return a.engine.Remove(executionID)
}

// ── 节点类型 ─────────────────────────────────────────────────

// GetNodeTypes 获取所有可用的节点类型定义
// 返回 = engine 注册表中的内置节点 + 当前所有集合动态生成的节点类型
func (a *App) GetNodeTypes() []core.NodeTypeDef {
	types := engine.AllTypeDefs()

	// 把每个已存在的集合转换成一个可调用的节点类型
	if a.assembleStore != nil {
		if assembles, err := a.assembleStore.List(); err == nil {
			for _, asm := range assembles {
				types = append(types, assembleToNodeType(asm))
			}
		}
	}
	return types
}

// assembleToNodeType 把集合定义转换为可调用的节点类型
// 端口 = exec_in/out + 每个 param/return 一个数据端口
func assembleToNodeType(asm core.AssembleDef) core.NodeTypeDef {
	inputPorts := []core.PortDef{
		{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
	}
	for _, p := range asm.Params {
		inputPorts = append(inputPorts, core.PortDef{
			ID:       "param_" + p.Name,
			Label:    p.Name,
			PortType: p.VarType,
		})
	}
	outputPorts := []core.PortDef{
		{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
	}
	for _, r := range asm.Returns {
		outputPorts = append(outputPorts, core.PortDef{
			ID:       "return_" + r.Name,
			Label:    r.Name,
			PortType: r.VarType,
		})
	}
	return core.NodeTypeDef{
		TypeID:        assembleTypePrefix + asm.ID,
		DisplayName:   asm.Name,
		Category:      "assemble_call",
		NodeKind:      core.NodeKindAction,
		Icon:          "📦",
		Description:   asm.Description,
		InputPorts:    inputPorts,
		OutputPorts:   outputPorts,
		ExecutionMode: core.ExecutionModeFlow,
	}
}

