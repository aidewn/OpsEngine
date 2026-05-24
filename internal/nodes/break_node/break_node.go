// break 节点：触发后终止整个工作流执行
// 行为等价于用户点击「停止」按钮：cancel runtime ctx → 跑 system_over → 标记 Terminated
//
// 包名用 break_node 避免与 Go 关键字 break 冲突
// 引擎特判处理（不走 Node.Execute），TypeID 为 "break"

package break_node

import (
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node break 节点实现
type Node struct{}

// TypeDef 节点元信息
// 1 exec_in，0 output（执行流的终点之一）
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "break",
		DisplayName: "Break",
		Category:    "flow",
		NodeKind:    core.NodeKindAction,
		Icon:        "⛔",
		Description: "终止整个工作流",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
		},
		OutputPorts:   []core.PortDef{},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute 由引擎特判处理，此处仅为接口契约保留
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return nil, nil
}
