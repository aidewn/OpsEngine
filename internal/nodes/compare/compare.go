// 比较节点：compare_eq / ne / lt / gt / le / ge
// 全部为 Pure 节点，按 config.var_type 选 Int / Float / String / Bool
// 端口 a / b 为 Dynamic（前端按 var_type 解析），result 固定为 Bool
//
// 类型支持：
//   eq / ne：Int / Float / String / Bool
//   lt / gt / le / ge：Int / Float / String（Bool 无序，不提供）
//
// 错误策略：
//   类型不可转时，转换 helper 返回零值（且打 warn）。pure 节点 Execute 不返回 error。

package compare

import (
	"strings"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	"OpsEngine/internal/nodes/exprhelper"
)

// 全部相等比较支持的类型
var equalityTypeOptions = []string{"Int", "Float", "String", "Bool"}

// 顺序比较支持的类型（Bool 无序）
var orderTypeOptions = []string{"Int", "Float", "String"}

func init() {
	engine.Register(&EqNode{})
	engine.Register(&NeNode{})
	engine.Register(&LtNode{})
	engine.Register(&GtNode{})
	engine.Register(&LeNode{})
	engine.Register(&GeNode{})
}

// ── 共用元数据构造 ────────────────────────────────────────

// binaryPorts 返回比较节点的端口（a, b 输入；result 固定 Bool 输出）
func binaryPorts() ([]core.PortDef, []core.PortDef) {
	in := []core.PortDef{
		{ID: "a", Label: "A", PortType: core.PortTypeDynamic, Required: true},
		{ID: "b", Label: "B", PortType: core.PortTypeDynamic, Required: true},
	}
	out := []core.PortDef{
		{ID: "result", Label: "结果", PortType: core.PortTypeBool},
	}
	return in, out
}

// varTypeField 类型 select 字段，与 var_set/var_get 同名
// allowed 决定 select options，default 为列表第一项
func varTypeField(allowed []string) core.FieldSchema {
	return core.FieldSchema{
		Type:    "select",
		ID:      "var_type",
		Label:   "值类型",
		Options: allowed,
		Default: allowed[0],
	}
}

// resolveType 读 config.var_type，缺省 / 不在 allowed 列表时回退到 allowed[0]
func resolveType(ctx engine.ExecContext, allowed []string) string {
	t := ctx.ConfigString("var_type")
	for _, a := range allowed {
		if a == t {
			return t
		}
	}
	return allowed[0]
}

// makeTypeDef 构造比较节点的通用 TypeDef
func makeTypeDef(typeID, name, icon, description string, allowed []string) core.NodeTypeDef {
	in, out := binaryPorts()
	return core.NodeTypeDef{
		TypeID:        typeID,
		DisplayName:   name,
		Category:      "compare",
		NodeKind:      core.NodeKindPure,
		Icon:          icon,
		Description:   description,
		InputPorts:    in,
		OutputPorts:   out,
		ConfigSchema:  []core.FieldSchema{varTypeField(allowed)},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// ── 求值 helper ──────────────────────────────────────────

func readPair(ctx engine.ExecContext) (any, any) {
	a, _ := ctx.Input("a")
	b, _ := ctx.Input("b")
	return a, b
}

// equalityResult 实现 eq/ne 共用逻辑，eqMode=true 返回 == 结果，false 返回 != 结果
func equalityResult(ctx engine.ExecContext, eqMode bool) bool {
	a, b := readPair(ctx)
	t := resolveType(ctx, equalityTypeOptions)
	var equal bool
	switch t {
	case "Float":
		av, _ := exprhelper.ToFloat64(a)
		bv, _ := exprhelper.ToFloat64(b)
		equal = av == bv
	case "String":
		av, _ := exprhelper.ToString(a)
		bv, _ := exprhelper.ToString(b)
		equal = av == bv
	case "Bool":
		av, _ := exprhelper.ToBool(a)
		bv, _ := exprhelper.ToBool(b)
		equal = av == bv
	default: // Int
		av, _ := exprhelper.ToInt64(a)
		bv, _ := exprhelper.ToInt64(b)
		equal = av == bv
	}
	if eqMode {
		return equal
	}
	return !equal
}

// orderResult 实现 lt/gt/le/ge 共用逻辑，op ∈ "<" ">" "<=" ">="
func orderResult(ctx engine.ExecContext, op string) bool {
	a, b := readPair(ctx)
	t := resolveType(ctx, orderTypeOptions)
	switch t {
	case "Float":
		av, _ := exprhelper.ToFloat64(a)
		bv, _ := exprhelper.ToFloat64(b)
		return cmpFloat(av, bv, op)
	case "String":
		av, _ := exprhelper.ToString(a)
		bv, _ := exprhelper.ToString(b)
		return cmpStr(av, bv, op)
	default: // Int
		av, _ := exprhelper.ToInt64(a)
		bv, _ := exprhelper.ToInt64(b)
		return cmpInt(av, bv, op)
	}
}

func cmpInt(a, b int64, op string) bool {
	switch op {
	case "<":
		return a < b
	case ">":
		return a > b
	case "<=":
		return a <= b
	case ">=":
		return a >= b
	}
	return false
}

func cmpFloat(a, b float64, op string) bool {
	switch op {
	case "<":
		return a < b
	case ">":
		return a > b
	case "<=":
		return a <= b
	case ">=":
		return a >= b
	}
	return false
}

func cmpStr(a, b, op string) bool {
	n := strings.Compare(a, b)
	switch op {
	case "<":
		return n < 0
	case ">":
		return n > 0
	case "<=":
		return n <= 0
	case ">=":
		return n >= 0
	}
	return false
}

// ── compare_eq ───────────────────────────────────────────

type EqNode struct{}

func (EqNode) TypeDef() core.NodeTypeDef {
	return makeTypeDef("compare_eq", "等于 (==)", "🟰",
		"返回 A == B；支持 Int / Float / String / Bool", equalityTypeOptions)
}

func (EqNode) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return engine.Outputs{"result": equalityResult(ctx, true)}, nil
}

// ── compare_ne ───────────────────────────────────────────

type NeNode struct{}

func (NeNode) TypeDef() core.NodeTypeDef {
	return makeTypeDef("compare_ne", "不等于 (!=)", "≠",
		"返回 A != B；支持 Int / Float / String / Bool", equalityTypeOptions)
}

func (NeNode) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return engine.Outputs{"result": equalityResult(ctx, false)}, nil
}

// ── compare_lt ───────────────────────────────────────────

type LtNode struct{}

func (LtNode) TypeDef() core.NodeTypeDef {
	return makeTypeDef("compare_lt", "小于 (<)", "<",
		"返回 A < B；支持 Int / Float / String", orderTypeOptions)
}

func (LtNode) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return engine.Outputs{"result": orderResult(ctx, "<")}, nil
}

// ── compare_gt ───────────────────────────────────────────

type GtNode struct{}

func (GtNode) TypeDef() core.NodeTypeDef {
	return makeTypeDef("compare_gt", "大于 (>)", ">",
		"返回 A > B；支持 Int / Float / String", orderTypeOptions)
}

func (GtNode) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return engine.Outputs{"result": orderResult(ctx, ">")}, nil
}

// ── compare_le ───────────────────────────────────────────

type LeNode struct{}

func (LeNode) TypeDef() core.NodeTypeDef {
	return makeTypeDef("compare_le", "小于等于 (<=)", "≤",
		"返回 A <= B；支持 Int / Float / String", orderTypeOptions)
}

func (LeNode) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return engine.Outputs{"result": orderResult(ctx, "<=")}, nil
}

// ── compare_ge ───────────────────────────────────────────

type GeNode struct{}

func (GeNode) TypeDef() core.NodeTypeDef {
	return makeTypeDef("compare_ge", "大于等于 (>=)", "≥",
		"返回 A >= B；支持 Int / Float / String", orderTypeOptions)
}

func (GeNode) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return engine.Outputs{"result": orderResult(ctx, ">=")}, nil
}
