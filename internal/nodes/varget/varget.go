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

// Execute Phase 0 暂时空实现，Phase 2 实装
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return nil, nil
}
