// var_set 写入当前 frame 变量的值（Action 节点）
// 输入端口 value 是 Dynamic，真实类型由 config.var_type 决定
// Phase 2 实装

package varset

import (
	"fmt"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node var_set 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
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
				Options: engine.VarTypeOptions, Default: "String"},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute 把 value input 写入当前 frame 的指定变量
// var_name 必填，否则报错
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	name := ctx.ConfigString("var_name")
	if name == "" {
		return nil, fmt.Errorf("var_set 节点的 var_name 未配置")
	}
	value, _ := ctx.Input("value")
	ctx.SetVariable(name, value)
	ctx.Info("设置变量 %s = %v", name, value)
	return nil, nil
}
