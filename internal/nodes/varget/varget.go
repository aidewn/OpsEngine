// var_get 读取当前 frame 变量的值（Pure 节点）
// 输出端口 value 是 Dynamic，真实类型由 config.var_type 决定
// Phase 2 实装

package varget

import (
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node var_get 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
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
				Options: engine.VarTypeOptions, Default: "String"},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute 从当前 frame 读变量并输出
// 未定义的变量返回 nil 并写 warn 日志（容错策略，跟未连线 input 行为一致）
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	name := ctx.ConfigString("var_name")
	if name == "" {
		return engine.Outputs{"value": nil}, nil
	}
	v, ok := ctx.GetVariable(name)
	if !ok {
		ctx.Warn("变量 %q 未定义，使用零值", name)
		return engine.Outputs{"value": nil}, nil
	}
	return engine.Outputs{"value": v}, nil
}
