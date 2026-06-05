// 服务器巡检的"运维计划"中间表示。
//
// 设计动机（文档 P6）：
//   - 模型直接拼工作流 JSON 不稳定（节点 id / 端口 / 字段易编造）。
//   - 两阶段生成：模型只产 Plan，后端把 Plan 翻译成 core.WorkflowDef。
//   - Plan 的结构很窄——只关心"要检查什么 + 用什么命令"，让模型容易输出。

package inspection

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Plan 是一份巡检计划，由 LLM 输出，后端消费。
// 与 workflow.Draft 的关键差别：Plan 不涉及节点 id / 坐标 / 端口，模型不会出错。
type Plan struct {
	// Name 用作生成工作流的 name。留空时由 materializer 补默认值。
	Name string `json:"name"`
	// Description 用作工作流描述。
	Description string `json:"description"`
	// Items 是巡检项列表，顺序就是工作流中命令的执行顺序。
	Items []Item `json:"items"`
}

// Item 是一条巡检项，对应工作流中的一个 linux_exec_command 节点。
type Item struct {
	// Title 是节点显示名，前端执行详情页会用它来呈现这一步在做什么。
	Title string `json:"title"`
	// Command 是要执行的 shell 命令，直接传给 bash -c。
	Command string `json:"command"`
	// TimeoutSeconds 是命令的执行超时，<=0 时由 materializer 填默认值。
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}

// MinItems / MaxItems 限制单次巡检的项数。
// 下限避免空巡检；上限避免一次性塞太多检查项导致执行链过长、调试困难。
const (
	MinItems = 1
	MaxItems = 20
)

// ParsePlan 从模型回复中提取并解析 JSON 巡检计划。
// 兼容模型常见的带 Markdown 围栏 / 前后解释输出，截取首个 { 到末尾 } 之间。
func ParsePlan(reply string) (Plan, error) {
	text := strings.TrimSpace(reply)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return Plan{}, errors.New("AI 返回内容不是 JSON")
	}
	var plan Plan
	if err := json.Unmarshal([]byte(text[start:end+1]), &plan); err != nil {
		return Plan{}, fmt.Errorf("解析巡检计划失败: %w", err)
	}
	if err := plan.Validate(); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// Validate 校验 Plan 的基础合法性。注意命令安全性校验在 materializer 中执行，
// 避免合法的 Plan 形状里隐藏破坏性命令。
func (p Plan) Validate() error {
	if len(p.Items) < MinItems {
		return fmt.Errorf("巡检计划至少需要 %d 项，实际 %d", MinItems, len(p.Items))
	}
	if len(p.Items) > MaxItems {
		return fmt.Errorf("巡检计划最多 %d 项，实际 %d", MaxItems, len(p.Items))
	}
	for i, it := range p.Items {
		if strings.TrimSpace(it.Title) == "" {
			return fmt.Errorf("巡检项 #%d 缺少 title", i+1)
		}
		if strings.TrimSpace(it.Command) == "" {
			return fmt.Errorf("巡检项 %q 缺少 command", it.Title)
		}
	}
	return nil
}
