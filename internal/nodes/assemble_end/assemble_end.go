// assemble_end 集合执行出口
// 单例，单 exec_in，无 exec_out
// Phase 4 实装：收集 input 端口值到当前 frame.Returns

package assemble_end

import (
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node assemble_end 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "assemble_end",
		DisplayName: "End",
		Category:    "assemble",
		NodeKind:    core.NodeKindAction,
		Icon:        "⏹️",
		Description: "集合执行出口",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
		},
		OutputPorts:   []core.PortDef{},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute Phase 0 暂时空实现，Phase 4 实装时遍历 returns 端口收集值
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return nil, nil
}
