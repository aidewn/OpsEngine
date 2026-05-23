// thread 线程节点：spawn 一个独立 goroutine 跑 exec_out_thread 链
// 主流立即沿 exec_out_continue 推进
// 线程的生命周期受工作流 context 管理（工作流停止时被 cancel）
// Phase 5 实装；Phase 0 仅注册 TypeDef

package thread

import (
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node thread 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "thread",
		DisplayName: "线程",
		Category:    "flow",
		NodeKind:    core.NodeKindFlowControl,
		Icon:        "⤴",
		Description: "spawn 独立线程执行分支，主流立即继续",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out_continue", Label: "▶ continue", PortType: core.PortTypeExec},
			{ID: "exec_out_thread", Label: "▶ thread", PortType: core.PortTypeExec},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute Phase 0 暂时空实现，Phase 5 实装
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return nil, nil
}
