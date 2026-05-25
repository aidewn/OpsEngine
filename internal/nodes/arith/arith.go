// 算术节点：math_add / math_sub / math_mul / math_div / math_mod
// 全部为 Pure 节点；按 config.var_type 选 Int 或 Float
// 端口 a / b / result 均为 Dynamic，前端按 var_type 解析真实类型
//
// 错误策略：
//   pure 节点 Execute 返回 error 会被 evaluator.evalOutput 静默吞掉，
//   下游 Input 拿不到值，难排查。所以这里所有"运算异常"（除零、类型不可转）
//   都返回零值 + ctx.Warn 日志，永不返回 error。

package arith

import (
	"math"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	"OpsEngine/internal/nodes/exprhelper"
)

// numberTypeOptions 算术节点支持的数值类型，与 var_set/var_get 共用 var_type 字段名
var numberTypeOptions = []string{"Int", "Float"}

// 注册全部算术节点
func init() {
	engine.Register(&AddNode{})
	engine.Register(&SubNode{})
	engine.Register(&MulNode{})
	engine.Register(&DivNode{})
	engine.Register(&ModNode{})
}

// ── 共用元数据构造 ────────────────────────────────────────

// binaryPorts 返回二元算术节点的端口（a, b 输入；result 输出）
func binaryPorts() ([]core.PortDef, []core.PortDef) {
	in := []core.PortDef{
		{ID: "a", Label: "A", PortType: core.PortTypeDynamic, Required: true},
		{ID: "b", Label: "B", PortType: core.PortTypeDynamic, Required: true},
	}
	out := []core.PortDef{
		{ID: "result", Label: "结果", PortType: core.PortTypeDynamic},
	}
	return in, out
}

// varTypeField 类型 select 字段，与 var_set/var_get 同名同语义
func varTypeField() core.FieldSchema {
	return core.FieldSchema{
		Type:    "select",
		ID:      "var_type",
		Label:   "数值类型",
		Options: numberTypeOptions,
		Default: "Int",
	}
}

// resolveType 读 config.var_type，缺省 / 非法值回退到 Int
func resolveType(ctx engine.ExecContext) string {
	if ctx.ConfigString("var_type") == "Float" {
		return "Float"
	}
	return "Int"
}

// readOperands 求两个操作数并按类型转换
// 转换失败时打 warn 日志（不阻断，使用零值继续）
func readOperands(ctx engine.ExecContext, numType string) (intA, intB int64, fA, fB float64) {
	a, _ := ctx.Input("a")
	b, _ := ctx.Input("b")

	if numType == "Float" {
		var ok bool
		if fA, ok = exprhelper.ToFloat64(a); !ok && a != nil {
			ctx.Warn("操作数 a (%v) 无法转为 Float，使用 0", a)
		}
		if fB, ok = exprhelper.ToFloat64(b); !ok && b != nil {
			ctx.Warn("操作数 b (%v) 无法转为 Float，使用 0", b)
		}
		return
	}

	var ok bool
	if intA, ok = exprhelper.ToInt64(a); !ok && a != nil {
		ctx.Warn("操作数 a (%v) 无法转为 Int，使用 0", a)
	}
	if intB, ok = exprhelper.ToInt64(b); !ok && b != nil {
		ctx.Warn("操作数 b (%v) 无法转为 Int，使用 0", b)
	}
	return
}

// makeTypeDef 构造算术节点的通用 TypeDef
func makeTypeDef(typeID, name, icon, description string) core.NodeTypeDef {
	in, out := binaryPorts()
	return core.NodeTypeDef{
		TypeID:        typeID,
		DisplayName:   name,
		Category:      "math",
		NodeKind:      core.NodeKindPure,
		Icon:          icon,
		Description:   description,
		InputPorts:    in,
		OutputPorts:   out,
		ConfigSchema:  []core.FieldSchema{varTypeField()},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// ── math_add ─────────────────────────────────────────────

type AddNode struct{}

func (AddNode) TypeDef() core.NodeTypeDef {
	return makeTypeDef("math_add", "加 (+)", "➕", "返回 A + B")
}

func (AddNode) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	t := resolveType(ctx)
	ai, bi, af, bf := readOperands(ctx, t)
	if t == "Float" {
		return engine.Outputs{"result": af + bf}, nil
	}
	return engine.Outputs{"result": ai + bi}, nil
}

// ── math_sub ─────────────────────────────────────────────

type SubNode struct{}

func (SubNode) TypeDef() core.NodeTypeDef {
	return makeTypeDef("math_sub", "减 (-)", "➖", "返回 A - B")
}

func (SubNode) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	t := resolveType(ctx)
	ai, bi, af, bf := readOperands(ctx, t)
	if t == "Float" {
		return engine.Outputs{"result": af - bf}, nil
	}
	return engine.Outputs{"result": ai - bi}, nil
}

// ── math_mul ─────────────────────────────────────────────

type MulNode struct{}

func (MulNode) TypeDef() core.NodeTypeDef {
	return makeTypeDef("math_mul", "乘 (×)", "✖️", "返回 A × B")
}

func (MulNode) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	t := resolveType(ctx)
	ai, bi, af, bf := readOperands(ctx, t)
	if t == "Float" {
		return engine.Outputs{"result": af * bf}, nil
	}
	return engine.Outputs{"result": ai * bi}, nil
}

// ── math_div ─────────────────────────────────────────────

type DivNode struct{}

func (DivNode) TypeDef() core.NodeTypeDef {
	return makeTypeDef("math_div", "除 (÷)", "➗", "返回 A / B；除零时返回 0 并打 warn")
}

func (DivNode) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	t := resolveType(ctx)
	ai, bi, af, bf := readOperands(ctx, t)
	if t == "Float" {
		if bf == 0 {
			ctx.Warn("除以零，返回 0")
			return engine.Outputs{"result": float64(0)}, nil
		}
		return engine.Outputs{"result": af / bf}, nil
	}
	if bi == 0 {
		ctx.Warn("除以零，返回 0")
		return engine.Outputs{"result": int64(0)}, nil
	}
	return engine.Outputs{"result": ai / bi}, nil
}

// ── math_mod ─────────────────────────────────────────────

type ModNode struct{}

func (ModNode) TypeDef() core.NodeTypeDef {
	return makeTypeDef("math_mod", "取模 (%)", "％", "返回 A % B；Float 类型也支持，模零时返回 0")
}

func (ModNode) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	t := resolveType(ctx)
	ai, bi, af, bf := readOperands(ctx, t)
	if t == "Float" {
		if bf == 0 {
			ctx.Warn("模零，返回 0")
			return engine.Outputs{"result": float64(0)}, nil
		}
		return engine.Outputs{"result": math.Mod(af, bf)}, nil
	}
	if bi == 0 {
		ctx.Warn("模零，返回 0")
		return engine.Outputs{"result": int64(0)}, nil
	}
	return engine.Outputs{"result": ai % bi}, nil
}
