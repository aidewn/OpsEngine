// print 调试节点：把 message 输入打印到执行日志
// Phase 2 实装

package print

import (
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	"OpsEngine/internal/nodes/exprhelper"
)

func init() { engine.Register(&Node{}) }

// Node print 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "print",
		DisplayName: "打印",
		Category:    "debug",
		NodeKind:    core.NodeKindAction,
		Icon:        "📝",
		Description: "打印消息到执行日志",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "message", Label: "消息", PortType: core.PortTypeAny},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "prefix", Label: "前缀", Placeholder: "[DEBUG]"},
			{Type: "textarea", ID: "default_text", Label: "默认文本",
				Placeholder: "message 未连接时使用此文本"},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute 把 message 输入打印到日志
// prefix 配置作为日志前缀，未配置时用 "[INFO]"
// 当 message input 未连接时，使用 default_text 作为消息内容
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	msg, ok := ctx.Input("message")
	if !ok {
		msg = ctx.ConfigString("default_text")
	}
	prefix := ctx.ConfigString("prefix")
	if prefix == "" {
		prefix = "[INFO]"
	}
	ctx.Info("%s %s", prefix, exprhelper.FormatValue(msg))
	return nil, nil
}
