// regex_extract 纯数据节点：对输入字符串执行正则提取，返回所有匹配项数组
// group=0 提取完整匹配；group=N 提取第 N 个捕获组
// 供下游 for_loop 迭代处理，同时提供 first_match 便捷端口用于单值场景

package regex_extract

import (
	"fmt"
	"regexp"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node regex_extract 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	minGroup := int64(0)
	return core.NodeTypeDef{
		TypeID:      "regex_extract",
		DisplayName: "正则提取",
		Category:    "data",
		NodeKind:    core.NodeKindPure,
		Icon:        "🔍",
		Description: "对输入字符串执行正则匹配，输出所有匹配项数组；group=0 取完整匹配，group=N 取第 N 捕获组",
		InputPorts: []core.PortDef{
			{ID: "text", Label: "文本", PortType: core.PortTypeString},
		},
		OutputPorts: []core.PortDef{
			{ID: "matches", Label: "匹配列表", PortType: core.PortTypeAny},
			{ID: "first_match", Label: "首个匹配", PortType: core.PortTypeString},
			{ID: "count", Label: "匹配数", PortType: core.PortTypeInt},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "pattern", Label: "正则表达式", Required: true,
				Placeholder: `\d+`},
			{Type: "number", ID: "group", Label: "捕获组编号（0 = 完整匹配）",
				Min: &minGroup, Default: int64(0)},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute 编译正则并提取所有匹配
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	textVal, _ := ctx.Input("text")
	text, _ := textVal.(string)

	pattern := ctx.ConfigString("pattern")
	if pattern == "" {
		return nil, fmt.Errorf("regex_extract 节点的 pattern 未配置")
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("正则编译失败: %w", err)
	}

	group := int(ctx.ConfigInt("group"))

	var matches []string
	if group == 0 {
		// 完整匹配，直接 FindAllString
		matches = re.FindAllString(text, -1)
	} else {
		// 捕获组，FindAllStringSubmatch 后提取第 group 项
		all := re.FindAllStringSubmatch(text, -1)
		for _, sub := range all {
			if group < len(sub) {
				matches = append(matches, sub[group])
			}
		}
	}

	if matches == nil {
		matches = []string{}
	}

	firstMatch := ""
	if len(matches) > 0 {
		firstMatch = matches[0]
	}

	return engine.Outputs{
		"matches":     matches,
		"first_match": firstMatch,
		"count":       int64(len(matches)),
	}, nil
}
