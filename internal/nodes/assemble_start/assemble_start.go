// assemble_start 集合执行入口
// 单例（每个集合最多 1 个），无 exec_in，单 exec_out
// Phase 4 实装：作为集合 frame 的流起点

package assemble_start

import (
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node assemble_start 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "assemble_start",
		DisplayName: "Start",
		Category:    "assemble",
		NodeKind:    core.NodeKindEvent,
		Icon:        "▶️",
		Description: "集合执行入口",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute 节点本身无逻辑，仅作为 frame 内 exec 流的起点
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return nil, nil
}
