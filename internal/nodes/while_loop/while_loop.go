// while_loop 条件循环节点：每次迭代前重新求 condition，true 跑 body，false 走 done
//
// 实际执行逻辑由引擎特判（internal/engine/while_loop.go），此处仅注册元数据
//
// 端口：
//   exec_in           : 触发
//   condition (Bool)  : 循环条件，每次迭代前重新求值
//   exec_out_body     : 每次迭代触发的分支
//   exec_out_done     : 循环结束后的主流出口
//
// 用户须知：condition 的依赖链应为 pure（如 compare_* + var_get），
// 否则 condition 会被 action 输出缓存固定，造成死循环。
// 在 body 内通过 var_set 更新 condition 依赖的变量来推进循环。

package while_loop

import (
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node while_loop 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "while_loop",
		DisplayName: "While 循环",
		Category:    "flow",
		NodeKind:    core.NodeKindFlowControl,
		Icon:        "🔄",
		Description: "每次迭代前重新求 condition，true 跑 body，false 走 done",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "condition", Label: "条件", PortType: core.PortTypeBool, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out_body", Label: "▶ body", PortType: core.PortTypeExec},
			{ID: "exec_out_done", Label: "▶ done", PortType: core.PortTypeExec},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute 由引擎特判处理（runWhile），此处仅为接口契约保留
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return nil, nil
}
