// to_string 纯数据节点：把任意类型输入格式化为 String 输出
// 与 print 共用 exprhelper.FormatValue；供需要显式 String 数据流的场景使用

package to_string

import (
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	"OpsEngine/internal/nodes/exprhelper"
)

func init() { engine.Register(&Node{}) }

// Node to_string 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "to_string",
		DisplayName: "转 String",
		Category:    "data",
		NodeKind:    core.NodeKindPure,
		Icon:        "🔤",
		Description: "把任意类型的值格式化为字符串",
		InputPorts: []core.PortDef{
			{ID: "value", Label: "值", PortType: core.PortTypeAny},
		},
		OutputPorts: []core.PortDef{
			{ID: "text", Label: "文本", PortType: core.PortTypeString},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute 格式化 value 输入为字符串
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	v, _ := ctx.Input("value")
	return engine.Outputs{"text": exprhelper.FormatValue(v)}, nil
}
