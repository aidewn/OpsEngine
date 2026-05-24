// assemble_param 输出集合的某个参数值（Pure 节点）
// 真实类型由 config.var_type 决定
// Phase 4 实装：从当前 frame.Params 读

package assemble_param

import (
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node assemble_param 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
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
				Options: engine.VarTypeOptions, Default: "String"},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute 从当前 frame.Params 读参数值并作为 output 输出
// 仅 assemble frame 有 params；主流 frame 调用此节点会输出 nil
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	name := ctx.ConfigString("param_name")
	if name == "" {
		return engine.Outputs{"value": nil}, nil
	}
	v, ok := ctx.GetParam(name)
	if !ok {
		ctx.Warn("参数 %q 未传入，使用零值", name)
		return engine.Outputs{"value": nil}, nil
	}
	return engine.Outputs{"value": v}, nil
}
