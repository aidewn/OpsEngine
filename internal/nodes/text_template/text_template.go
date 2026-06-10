// text_template 节点：用 {{ name }} 占位符把动态输入参数拼到模板文本里
//
// 配置：
//   - params: 用户定义的输入参数名列表（字符串数组），驱动前端动态生成 param_<name> input 端口
//   - template: 模板文本（多行），用 {{ name }} 引用参数
//
// 执行：扫描模板里所有 {{ name }} 占位符，按名字到 ctx.Input("param_<name>") 取值，
// 用 fmt.Sprint 字符串化。未连线 / 取不到的参数替换为空串并 Warn。

package text_template

import (
	"fmt"
	"regexp"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node text_template 节点实现
type Node struct{}

// templateVarRe 匹配 {{ name }} 占位符
// 允许 {{name}} / {{ name }} / {{  name  }} 等空白变体；变量名仅允许字母 / 数字 / 下划线，且不能数字开头
var templateVarRe = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "text_template",
		DisplayName: "文本模板",
		Category:    "data",
		NodeKind:    core.NodeKindPure,
		Icon:        "📝",
		Description: "用 {{ name }} 占位符把参数拼接到模板文本",
		// 输入端口由前端按 config.params 动态渲染；这里留空，引擎按 param_<name> 读取
		InputPorts: []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "text", Label: "文本", PortType: core.PortTypeString},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "param_list", ID: "params", Label: "输入参数",
				Placeholder: "每行一个参数名"},
			{Type: "textarea", ID: "template", Label: "模板",
				Placeholder: "示例：Hello {{ name }}, 来自 {{ host }}"},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute 按模板渲染输出
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	tpl := ctx.ConfigString("template")
	rendered := templateVarRe.ReplaceAllStringFunc(tpl, func(match string) string {
		name := templateVarRe.FindStringSubmatch(match)[1]
		v, ok := ctx.Input("param_" + name)
		if !ok || v == nil {
			ctx.Warn("模板参数 %q 未连接，使用空字符串", name)
			return ""
		}
		return fmt.Sprint(v)
	})
	return engine.Outputs{"text": rendered}, nil
}
