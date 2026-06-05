// Prompt 模板加载与渲染层。所有静态提示词来自项目根目录 prompts/，通过 embed 打包，
// 这样运维同学可以直接评审 Markdown 文件，而不需要看 Go 源码。
//
// 模板使用 Go text/template 语法，变量名直接对应外部传入的字段。

package prompt

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed all:files
var files embed.FS

// templateFile 是相对 files/ 的模板路径。新增模板时只需在此登记。
const (
	templateSystemOpsAssistant = "files/system/ops_assistant.md"
	templateWorkflowGeneration = "files/workflow/generation.md"
	templateInspectionPlan     = "files/workflow/inspection_plan.md"
	templateInspectionRisks    = "files/report/inspection_risks.md"
)

// renderTemplate 加载并执行指定模板，缺失变量时返回包含模板名的明确错误。
// 这里每次重新解析以保证模板修改后下次调用立即生效（构建期 embed 已绑定，运行期不会真正变）。
func renderTemplate(name string, data any) (string, error) {
	raw, err := files.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("加载 prompt 模板 %s 失败: %w", name, err)
	}
	tmpl, err := template.New(name).Option("missingkey=error").Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("解析 prompt 模板 %s 失败: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染 prompt 模板 %s 失败: %w", name, err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}
