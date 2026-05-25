// branch 条件分支节点：按 condition 输入选择走 true / false 出口
//
// 包名用 branch_node 避免被误以为与并发分支相关；TypeID 仍为 "branch"
// 实际执行逻辑由引擎特判（internal/engine/branch.go），此处仅注册元数据
//
// 端口：
//   exec_in           : 触发
//   condition (Bool)  : 条件值
//   exec_out_true     : condition 为真走这条
//   exec_out_false    : condition 为假走这条
//
// 容错：condition 未连接 / 类型不可识别时按 false 处理（与未连数据 input 的一贯策略一致）

package branch_node

import (
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node branch 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "branch",
		DisplayName: "条件分支",
		Category:    "flow",
		NodeKind:    core.NodeKindFlowControl,
		Icon:        "⑂",
		Description: "按 condition 选择走 true 或 false 分支",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "condition", Label: "条件", PortType: core.PortTypeBool, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out_true", Label: "▶ true", PortType: core.PortTypeExec},
			{ID: "exec_out_false", Label: "▶ false", PortType: core.PortTypeExec},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute 由引擎特判处理（runBranch），此处仅为接口契约保留
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return nil, nil
}
