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
		DisplayName: "Get 参数",
		Category:    "assemble",
		NodeKind:    core.NodeKindPure,
		Icon:        "📤",
		Description: "读取集合入参（调用方通过 param 端口传入）",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "value", Label: "值", PortType: core.PortTypeDynamic},
		},
		ConfigSchema: []core.FieldSchema{
			// param_select：前端从当前集合 Params 列表挑选；选中时同步 var_type
			{Type: "param_select", ID: "param_name", Label: "参数", Required: true},
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
