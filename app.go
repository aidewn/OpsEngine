// 桌面应用主结构，所有 public 方法自动绑定到前端 JS

package main

import (
	"context"
	"os"

	"OpsEngine/internal/core"
	"OpsEngine/internal/store"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// App 桌面应用实例，public 方法通过 Wails 绑定暴露给前端调用
type App struct {
	ctx           context.Context
	workflowStore *store.WorkflowStore
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
	for _, dir := range []string{"data/workflows", "data/executions", "data/logs"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			zap.L().Fatal("创建目录失败", zap.Error(err))
		}
	}

	a.workflowStore = store.NewWorkflowStore("data/workflows")
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
func (a *App) UpdateWorkflow(wf core.WorkflowDef) error {
	return a.workflowStore.Save(wf)
}

// DeleteWorkflow 删除工作流
func (a *App) DeleteWorkflow(id string) error {
	return a.workflowStore.Delete(id)
}

// ── 节点类型 ─────────────────────────────────────────────────

// GetNodeTypes 获取所有可用的节点类型定义
// 暂时硬编码，后续抽取到 registry
func (a *App) GetNodeTypes() []core.NodeTypeDef {
	return builtinNodeTypes
}

// 内置节点类型定义
var builtinNodeTypes = []core.NodeTypeDef{
	// ── 事件源节点（Event）───────────────────────────
	{
		TypeID:      "system_ready",
		DisplayName: "System Ready",
		Category:    "event",
		NodeKind:    core.NodeKindEvent,
		Icon:        "🟢",
		Description: "工作流启动时触发一次",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
		},
		ExecutionMode: core.ExecutionModeFlow,
	},
	{
		TypeID:      "system_update",
		DisplayName: "System Update",
		Category:    "event",
		NodeKind:    core.NodeKindEvent,
		Icon:        "🔵",
		Description: "按 delta 周期循环触发",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "select", ID: "delta_type", Label: "触发方式",
				Options: []string{"interval", "cron", "manual"}, Default: "interval"},
			{Type: "number", ID: "delta_seconds", Label: "间隔（秒）", Default: int64(60)},
			{Type: "text", ID: "cron_expr", Label: "Cron 表达式", Placeholder: "0 */5 * * *"},
		},
		ExecutionMode: core.ExecutionModeFlow,
	},
	{
		TypeID:      "system_over",
		DisplayName: "System Over",
		Category:    "event",
		NodeKind:    core.NodeKindEvent,
		Icon:        "🔴",
		Description: "工作流终止时触发一次",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
		},
		ExecutionMode: core.ExecutionModeFlow,
	},

	// ── 调试节点（Action）───────────────────────────
	{
		TypeID:      "print",
		DisplayName: "打印",
		Category:    "debug",
		NodeKind:    core.NodeKindAction,
		Icon:        "📝",
		Description: "打印消息到执行日志",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "message", Label: "消息", PortType: core.PortTypeString},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "prefix", Label: "前缀", Placeholder: "[DEBUG]"},
		},
		ExecutionMode: core.ExecutionModeFlow,
	},

	// ── 数据节点（Pure）─────────────────────────────
	{
		TypeID:      "variable",
		DisplayName: "变量",
		Category:    "data",
		NodeKind:    core.NodeKindPure,
		Icon:        "📦",
		Description: "定义一个带类型的变量",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "value", Label: "值", PortType: core.PortTypeDynamic},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "var_name", Label: "变量名", Required: true},
			{Type: "select", ID: "var_type", Label: "类型",
				Options: []string{"String", "LinuxSshConnection", "DockerContext", "K8sContext", "NginxInstance"},
				Default: "String"},
			{Type: "text", ID: "var_value", Label: "值", Required: true},
		},
		ExecutionMode: core.ExecutionModeFlow,
	},
}
