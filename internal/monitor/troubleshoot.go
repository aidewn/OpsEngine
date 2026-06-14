// Troubleshoot Flow（Phase 4/5）：异常后的低频排查，生成诊断报告。
//
//   - 事实层：异常触发证据 + 本轮重新采集的数据（始终可用）；
//   - 诊断层（Phase 5）：可选 AI 根因推断与修复建议，通过 Summarizer 注入；
//     未配置 AI（summarize 为 nil）或调用失败时优雅降级为纯事实报告。

package monitor

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
)

// Summarizer 把一组 chat messages 发给 LLM 拿回复，由调用方注入（与 ai.go 的 llmAdapter.Chat 同签名）。
type Summarizer func(messages []clients.ChatMessage) (string, error)

// TroubleshootReport 是排查流程的产物（标题/摘要/正文），由上层落成 MonitorReport。
type TroubleshootReport struct {
	Title   string
	Summary string
	Content string
}

// BuildTroubleshootReport 根据监控项、异常事件与本轮采集切片生成诊断报告。
// summarize 非空时追加 AI 诊断结论；为空则只产出事实报告。
func BuildTroubleshootReport(
	panel core.MonitorPanel,
	inc core.Incident,
	batch Batch,
	summarize Summarizer,
) TroubleshootReport {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s 诊断报告\n\n", panel.Name)
	fmt.Fprintf(&b, "- 监控项：%s\n", panel.Name)
	fmt.Fprintf(&b, "- 严重级别：%s\n", inc.Severity)
	fmt.Fprintf(&b, "- 异常开始：%s\n\n", inc.StartedAt.Format(time.RFC3339))

	b.WriteString("## 异常触发证据\n\n")
	if len(inc.Evidence) == 0 {
		b.WriteString("（无）\n\n")
	} else {
		for _, e := range inc.Evidence {
			fmt.Fprintf(&b, "- %s\n", e)
		}
		b.WriteString("\n")
	}

	b.WriteString("## 当前采集数据\n\n")
	if len(batch) == 0 {
		b.WriteString("（本轮未采集到数据）\n\n")
	} else {
		for _, res := range batch {
			fmt.Fprintf(&b, "### %s（target: %s）\n\n", res.Task.Kind, res.Task.TargetID)
			if res.Err != nil {
				fmt.Fprintf(&b, "采集失败：%v\n\n", res.Err)
				continue
			}
			fmt.Fprintf(&b, "```json\n%s\n```\n\n", prettyJSON(res.Data))
		}
	}

	b.WriteString("## 诊断结论\n\n")
	b.WriteString(diagnosisSection(panel, inc, batch, summarize))
	b.WriteString("\n")

	summary := inc.Title
	if summary == "" {
		summary = panel.Name + " 异常诊断"
	}
	return TroubleshootReport{
		Title:   panel.Name + " 诊断报告",
		Summary: summary,
		Content: b.String(),
	}
}

// diagnosisSection 产出诊断结论段：有 AI 则用 AI，否则给出降级说明。
func diagnosisSection(panel core.MonitorPanel, inc core.Incident, batch Batch, summarize Summarizer) string {
	if summarize == nil {
		return "（未配置 AI，已略过根因推断；在 AI 设置中配置后重新诊断即可获得分析与建议）"
	}
	text, err := summarize([]clients.ChatMessage{
		{Role: "system", Content: diagnosisSystemPrompt},
		{Role: "user", Content: renderFactsForLLM(panel, inc, batch)},
	})
	if err != nil {
		// AI 失败不致命：事实层已可用，把错误作为提示写入。
		return fmt.Sprintf("> AI 诊断失败：%s\n>\n> 以上事实与证据仍可用于人工排查。", err.Error())
	}
	return strings.TrimSpace(text)
}

// diagnosisSystemPrompt 约束 AI 只基于给定事实输出，避免编造。
const diagnosisSystemPrompt = "你是运维诊断助手。基于给定的监控异常证据与当前采集数据，输出简洁的中文 Markdown，" +
	"包含四部分：1) 最可能的根因；2) 置信度（高/中/低）；3) 排查与修复建议（分点）；4) 风险提示。" +
	"只依据提供的数据推断，不要编造未给出的指标或事实；信息不足时明确指出还需补充哪些数据。"

// renderFactsForLLM 把监控项、异常与采集数据压成 LLM 易消费的事实摘要。
func renderFactsForLLM(panel core.MonitorPanel, inc core.Incident, batch Batch) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "监控项：%s\n", panel.Name)
	if panel.Description != "" {
		fmt.Fprintf(&sb, "说明：%s\n", panel.Description)
	}
	fmt.Fprintf(&sb, "严重级别：%s\n\n", inc.Severity)

	sb.WriteString("触发证据：\n")
	if len(inc.Evidence) == 0 {
		sb.WriteString("（无）\n")
	} else {
		for _, e := range inc.Evidence {
			fmt.Fprintf(&sb, "- %s\n", e)
		}
	}
	sb.WriteString("\n当前采集数据：\n")
	if len(batch) == 0 {
		sb.WriteString("（无）\n")
	} else {
		for _, res := range batch {
			if res.Err != nil {
				fmt.Fprintf(&sb, "- %s 采集失败：%v\n", res.Task.Kind, res.Err)
				continue
			}
			fmt.Fprintf(&sb, "- %s: %s\n", res.Task.Kind, compactJSON(res.Data))
		}
	}
	return sb.String()
}

// prettyJSON 把采集数据格式化为可读 JSON，失败时降级为 Go 默认表示。
func prettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// compactJSON 单行 JSON，用于喂给 LLM 的紧凑事实。
func compactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
