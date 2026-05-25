// for_loop 计数循环节点：[start, end) 半开区间，每次迭代触发 exec_out_body
// 全部迭代完成后走 exec_out_done；index 输出端口反映当前迭代值
//
// 实际执行逻辑由引擎特判（internal/engine/for_loop.go），此处仅注册元数据
//
// 端口：
//   exec_in           : 触发
//   start (Int)       : 起始（含），未连按 0 处理
//   end   (Int)       : 结束（不含），未连按 0 处理 → 不进入 body
//   exec_out_body     : 每次迭代触发的分支
//   exec_out_done     : 循环结束后的主流出口
//   index (Int)       : 当前迭代值，body 内可读

package for_loop

import (
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node for_loop 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "for_loop",
		DisplayName: "For 循环",
		Category:    "flow",
		NodeKind:    core.NodeKindFlowControl,
		Icon:        "🔁",
		Description: "对 [start, end) 区间循环执行 body 分支，结束走 done",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "start", Label: "起始", PortType: core.PortTypeInt},
			{ID: "end", Label: "结束", PortType: core.PortTypeInt},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out_body", Label: "▶ body", PortType: core.PortTypeExec},
			{ID: "exec_out_done", Label: "▶ done", PortType: core.PortTypeExec},
			{ID: "index", Label: "index", PortType: core.PortTypeInt},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute 由引擎特判处理（runFor），此处仅为接口契约保留
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return nil, nil
}
