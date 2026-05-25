// return_set 写入集合返回值（Action 节点）
// 与 var_set 对称：侧栏 Returns 定义名称与类型，画布上拖入本节点赋值
// 真实类型由 config.var_type 决定；引擎写入当前 frame.Returns

package return_set

import (
	"fmt"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node return_set 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "return_set",
		DisplayName: "Set 返回值",
		Category:    "assemble",
		NodeKind:    core.NodeKindAction,
		Icon:        "📥",
		Description: "设置集合的返回值（调用方通过 return 端口读出）",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "value", Label: "值", PortType: core.PortTypeDynamic},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
		},
		ConfigSchema: []core.FieldSchema{
			// return_select：前端从当前集合 Returns 列表挑选；选中时同步 var_type
			{Type: "return_select", ID: "return_name", Label: "返回值", Required: true},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute 把 value input 写入当前 frame 的 Returns
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	name := ctx.ConfigString("return_name")
	if name == "" {
		return nil, fmt.Errorf("return_set 节点的 return_name 未配置")
	}
	value, _ := ctx.Input("value")
	ctx.SetReturn(name, value)
	ctx.Info("设置返回值 %s = %v", name, value)
	return nil, nil
}
