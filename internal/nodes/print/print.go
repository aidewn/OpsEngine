// print 调试节点：把 message 输入打印到执行日志
// Phase 2 实装

package print

import (
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
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
			{ID: "message", Label: "消息", PortType: core.PortTypeString},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "prefix", Label: "前缀", Placeholder: "[DEBUG]"},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute Phase 0 暂时空实现，Phase 2 实装
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return nil, nil
}
