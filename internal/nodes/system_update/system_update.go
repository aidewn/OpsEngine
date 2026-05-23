// system_update 工作流周期触发入口
// 单例，无 exec_in，单 exec_out
// 真正的周期调度逻辑由 Engine.scheduler 处理（Phase 6）

package system_update

import (
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node system_update 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "system_update",
		DisplayName: "System Update",
		Category:    "event",
		NodeKind:    core.NodeKindEvent,
		Icon:        "🔵",
		Description: "按 delta 周期循环触发",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "select", ID: "delta_type", Label: "触发方式",
				Options: []string{"interval", "cron", "manual"}, Default: "interval"},
			{Type: "number", ID: "delta_seconds", Label: "间隔（秒）", Default: int64(60)},
			{Type: "text", ID: "cron_expr", Label: "Cron 表达式", Placeholder: "0 */5 * * *"},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute 节点本身无逻辑（仅作为 update 阶段流起点）
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return nil, nil
}
