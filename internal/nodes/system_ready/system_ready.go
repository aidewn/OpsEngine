// system_ready 工作流启动入口
// 单例（每个工作流最多 1 个），无 exec_in，单 exec_out

package system_ready

import (
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node system_ready 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
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
	}
}

// Execute 事件源节点本身无逻辑，作为 exec 流起点
// Phase 2 实装时可能保留为空
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return nil, nil
}
