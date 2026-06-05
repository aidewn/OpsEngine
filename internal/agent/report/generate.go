// 巡检报告的高层编排：事实提取 → 可选 LLM 风险分析 → 渲染 → 构造 OpsDoc。
// 不直接依赖 LLM 客户端，通过 Summarizer 回调注入，方便测试与替换实现。

package report

import (
	"fmt"
	"strings"
	"time"

	"OpsEngine/internal/agent/prompt"
	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"

	"github.com/google/uuid"
)

// Summarizer 由调用方注入，把一组 chat messages 发给 LLM 拿到回复。
// 与 runtime.LLMProvider.Chat 签名一致，ai.go 用同一个 llmAdapter 适配两边。
type Summarizer func(messages []clients.ChatMessage) (string, error)

// GenerateInspection 产出一份 OpsDoc。Summarizer 可为 nil（不做 LLM 风险分析）。
// session 用于回写 Source.SessionID；可为零值。
func GenerateInspection(
	env core.EnvironmentDef,
	wf core.WorkflowDef,
	rec core.ExecutionRecord,
	session core.AISession,
	summarize Summarizer,
) (core.OpsDoc, error) {
	report := ExtractFacts(env, wf, rec)

	riskSection := ""
	if summarize != nil {
		text, err := summarizeRisks(report, summarize)
		if err != nil {
			// 风险分析失败不致命：报告事实层已经可用，把错误信息作为风险段告知用户。
			riskSection = fmt.Sprintf("> LLM 风险分析失败：%s\n>\n> 报告事实部分仍然可用，可稍后重新生成。", err.Error())
		} else {
			riskSection = strings.TrimSpace(text)
		}
	}

	body := Render(report, riskSection)
	now := time.Now()
	return core.OpsDoc{
		ID:    uuid.New().String(),
		Kind:  core.OpsDocKindInspection,
		Title: fmt.Sprintf("巡检报告：%s", report.WorkflowName),
		Source: core.OpsDocSource{
			EnvironmentID: env.ID,
			WorkflowID:    wf.ID,
			ExecutionID:   rec.ID,
			SessionID:     session.ID,
		},
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// summarizeRisks 用 LLM 把事实摘要分析成风险与建议段。
func summarizeRisks(r InspectionReport, summarize Summarizer) (string, error) {
	systemPrompt, err := prompt.BuildInspectionRisksPrompt(renderFactsForLLM(r))
	if err != nil {
		return "", err
	}
	return summarize([]clients.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "请基于以上事实输出风险与建议段落。"},
	})
}

// renderFactsForLLM 把 InspectionReport 压成 LLM 容易消费的事实摘要。
// 与 Render 不同：不带 Markdown 标题，每条更紧凑，便于模型聚焦事实。
func renderFactsForLLM(r InspectionReport) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "环境=%s, 工作流=%s, 总体状态=%s\n", r.EnvironmentName, r.WorkflowName, r.OverallStatus)
	fmt.Fprintf(&sb, "项数：")
	for k, v := range r.Counts() {
		fmt.Fprintf(&sb, "%s=%d ", stateLabel(k), v)
	}
	sb.WriteString("\n\n")
	for i, it := range r.Items {
		fmt.Fprintf(&sb, "## 项 %d - %s [%s]\n", i+1, it.Title, it.State)
		if it.Command != "" {
			fmt.Fprintf(&sb, "命令: %s\n", it.Command)
		}
		if len(it.Logs) == 0 {
			sb.WriteString("（无日志）\n")
		} else {
			sb.WriteString("日志:\n")
			for _, line := range it.Logs {
				sb.WriteString("  ")
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
