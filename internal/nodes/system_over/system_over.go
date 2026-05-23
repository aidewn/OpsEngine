// system_over 工作流终止钩子入口
// 单例，无 exec_in，单 exec_out
// Phase 6 实装：工作流被停止时触发这条流

package system_over

import (
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node system_over 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
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
	}
}

// Execute 节点本身无逻辑（仅作为 over 阶段流起点）
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return nil, nil
}
