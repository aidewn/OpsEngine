// 桌面应用主结构，所有 public 方法自动绑定到前端 JS

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"OpsEngine/internal/core"
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
// 保存前检查循环引用（直接或间接引用自身）
func (a *App) UpdateAssemble(a2 core.AssembleDef) error {
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

// ── 节点类型 ─────────────────────────────────────────────────

// GetNodeTypes 获取所有可用的节点类型定义
// 返回 = 内置节点 + 当前所有集合动态生成的节点类型
func (a *App) GetNodeTypes() []core.NodeTypeDef {
	types := make([]core.NodeTypeDef, 0, len(builtinNodeTypes))
	types = append(types, builtinNodeTypes...)

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

	// ── 变量节点 ────────────────────────────────────

	// var_get 读取变量值（Pure，无执行流）
	{
		TypeID:      "var_get",
		DisplayName: "Get 变量",
		Category:    "data",
		NodeKind:    core.NodeKindPure,
		Icon:        "📤",
		Description: "读取工作流/集合变量的当前值",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "value", Label: "值", PortType: core.PortTypeDynamic},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "var_name", Label: "变量名", Required: true},
			{Type: "select", ID: "var_type", Label: "类型",
				Options: []string{"String", "Int", "Float", "Bool", "LinuxSshConnection", "DockerContext", "K8sContext", "NginxInstance"},
				Default: "String"},
		},
		ExecutionMode: core.ExecutionModeFlow,
	},

	// var_set 写入变量值（Action，有执行流）
	{
		TypeID:      "var_set",
		DisplayName: "Set 变量",
		Category:    "data",
		NodeKind:    core.NodeKindAction,
		Icon:        "📥",
		Description: "设置工作流/集合变量的值",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "value", Label: "值", PortType: core.PortTypeDynamic},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "var_name", Label: "变量名", Required: true},
			{Type: "select", ID: "var_type", Label: "类型",
				Options: []string{"String", "Int", "Float", "Bool", "LinuxSshConnection", "DockerContext", "K8sContext", "NginxInstance"},
				Default: "String"},
		},
		ExecutionMode: core.ExecutionModeFlow,
	},

	// ── 集合内部节点 ────────────────────────────────

	// assemble_start 集合执行入口
	{
		TypeID:      "assemble_start",
		DisplayName: "Start",
		Category:    "assemble",
		NodeKind:    core.NodeKindEvent,
		Icon:        "▶️",
		Description: "集合执行入口",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
		},
		ExecutionMode: core.ExecutionModeFlow,
	},

	// assemble_end 集合执行出口
	{
		TypeID:      "assemble_end",
		DisplayName: "End",
		Category:    "assemble",
		NodeKind:    core.NodeKindAction,
		Icon:        "⏹️",
		Description: "集合执行出口",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
		},
		OutputPorts:   []core.PortDef{},
		ExecutionMode: core.ExecutionModeFlow,
	},

	// assemble_param 集合参数输出（Pure，由左侧面板拖入）
	{
		TypeID:      "assemble_param",
		DisplayName: "参数",
		Category:    "assemble",
		NodeKind:    core.NodeKindPure,
		Icon:        "📎",
		Description: "输出集合的一个参数值",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "value", Label: "值", PortType: core.PortTypeDynamic},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "param_name", Label: "参数名", Required: true},
			{Type: "select", ID: "var_type", Label: "类型",
				Options: []string{"String", "Int", "Float", "Bool", "LinuxSshConnection", "DockerContext", "K8sContext", "NginxInstance"},
				Default: "String"},
		},
		ExecutionMode: core.ExecutionModeFlow,
	},
}
