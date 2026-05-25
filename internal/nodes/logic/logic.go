// 逻辑节点：logic_and / logic_or / logic_not
// 全部为 Pure 节点；输入输出固定为 Bool（无需 var_type 配置）
//
// 输入未连或非 Bool 类型，会通过 exprhelper.ToBool 容错转换；
// 真正不可识别（如自定义 struct）按 false 处理。

package logic

import (
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	"OpsEngine/internal/nodes/exprhelper"
)

func init() {
	engine.Register(&AndNode{})
	engine.Register(&OrNode{})
	engine.Register(&NotNode{})
}

// ── 共用元数据构造 ────────────────────────────────────────

func binaryBoolPorts() ([]core.PortDef, []core.PortDef) {
	in := []core.PortDef{
		{ID: "a", Label: "A", PortType: core.PortTypeBool, Required: true},
		{ID: "b", Label: "B", PortType: core.PortTypeBool, Required: true},
	}
	out := []core.PortDef{
		{ID: "result", Label: "结果", PortType: core.PortTypeBool},
	}
	return in, out
}

func makeBinaryDef(typeID, name, icon, description string) core.NodeTypeDef {
	in, out := binaryBoolPorts()
	return core.NodeTypeDef{
		TypeID:        typeID,
		DisplayName:   name,
		Category:      "logic",
		NodeKind:      core.NodeKindPure,
		Icon:          icon,
		Description:   description,
		InputPorts:    in,
		OutputPorts:   out,
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// readBoolPair 读 a / b 输入并容错转 bool
func readBoolPair(ctx engine.ExecContext) (bool, bool) {
	a, _ := ctx.Input("a")
	b, _ := ctx.Input("b")
	av, _ := exprhelper.ToBool(a)
	bv, _ := exprhelper.ToBool(b)
	return av, bv
}

// ── logic_and ────────────────────────────────────────────

type AndNode struct{}

func (AndNode) TypeDef() core.NodeTypeDef {
	return makeBinaryDef("logic_and", "与 (AND)", "∧", "返回 A AND B")
}

func (AndNode) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	a, b := readBoolPair(ctx)
	return engine.Outputs{"result": a && b}, nil
}

// ── logic_or ─────────────────────────────────────────────

type OrNode struct{}

func (OrNode) TypeDef() core.NodeTypeDef {
	return makeBinaryDef("logic_or", "或 (OR)", "∨", "返回 A OR B")
}

func (OrNode) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	a, b := readBoolPair(ctx)
	return engine.Outputs{"result": a || b}, nil
}

// ── logic_not ────────────────────────────────────────────

type NotNode struct{}

func (NotNode) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "logic_not",
		DisplayName: "非 (NOT)",
		Category:    "logic",
		NodeKind:    core.NodeKindPure,
		Icon:        "¬",
		Description: "返回 !A",
		InputPorts: []core.PortDef{
			{ID: "a", Label: "A", PortType: core.PortTypeBool, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "result", Label: "结果", PortType: core.PortTypeBool},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

func (NotNode) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	a, _ := ctx.Input("a")
	av, _ := exprhelper.ToBool(a)
	return engine.Outputs{"result": !av}, nil
}
