// 从 AI 会话沉淀 OpsDoc。
// 与 inspection 报告不同：这里不消费 ExecutionRecord，而是把一条 assistant 消息（含工具调用 progress 与正文）
// 渲染成 Markdown 报告。适用于排障 / chat 等"对话产生分析结论"的场景。

package report

import (
	"fmt"
	"strings"
	"time"

	"OpsEngine/internal/core"

	"github.com/google/uuid"
)

// FromSessionMessage 把一条 assistant 消息渲染为 OpsDoc。
//   - kind 由消息的 Intent 决定（外层 ai.go 完成映射）
//   - 报告头记录环境/会话/原消息 ID，方便从文档库追溯回会话
//   - 工具调用 progress 单独成段，帮助读者理解结论是怎么得来的
//
// Returns 一个未持久化的 OpsDoc；调用方负责调用 OpsDocStore.Save。
func FromSessionMessage(
	env core.EnvironmentDef,
	session core.AISession,
	message core.AISessionMessage,
	kind core.OpsDocKind,
) (core.OpsDoc, error) {
	if message.Role != core.AIMessageRoleAssistant {
		return core.OpsDoc{}, fmt.Errorf("只有 assistant 消息可以保存为文档")
	}
	if strings.TrimSpace(message.Content) == "" {
		return core.OpsDoc{}, fmt.Errorf("消息正文为空，无法生成报告")
	}

	title := makeReportTitle(kind, session, message)
	body := renderSessionReport(env, session, message, title)
	now := time.Now()
	return core.OpsDoc{
		ID:    uuid.New().String(),
		Kind:  kind,
		Title: title,
		Source: core.OpsDocSource{
			EnvironmentID: env.ID,
			SessionID:     session.ID,
		},
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// makeReportTitle 按 kind 给出默认标题：
//   - troubleshooting：取会话标题 + 时间戳
//   - 其他：取 kindLabel + 会话标题
func makeReportTitle(kind core.OpsDocKind, session core.AISession, msg core.AISessionMessage) string {
	stamp := msg.CreatedAt
	if stamp.IsZero() {
		stamp = time.Now()
	}
	prefix := kindLabel(kind)
	sessionTitle := strings.TrimSpace(session.Title)
	if sessionTitle == "" {
		sessionTitle = "未命名会话"
	}
	return fmt.Sprintf("%s报告：%s（%s）", prefix, sessionTitle, stamp.Format("2006-01-02 15:04"))
}

// renderSessionReport 渲染从会话消息派生的 Markdown 报告。
func renderSessionReport(env core.EnvironmentDef, session core.AISession, msg core.AISessionMessage, title string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", title)
	sb.WriteString("## 来源\n\n")
	if name := strings.TrimSpace(env.Name); name != "" {
		fmt.Fprintf(&sb, "- 环境：%s\n", name)
	} else if env.ID != "" {
		fmt.Fprintf(&sb, "- 环境：`%s`\n", env.ID)
	}
	fmt.Fprintf(&sb, "- 会话：%s（`%s`）\n", session.Title, session.ID)
	fmt.Fprintf(&sb, "- 消息 ID：`%s`\n", msg.ID)
	if !msg.CreatedAt.IsZero() {
		fmt.Fprintf(&sb, "- 时间：%s\n", msg.CreatedAt.Format(time.RFC3339))
	}
	sb.WriteString("\n")

	if len(msg.Progress) > 0 {
		sb.WriteString("## 采集过程\n\n")
		sb.WriteString("以下是 Agent 在本次回答中执行的工具调用与状态变化（按时间顺序）：\n\n")
		for _, line := range msg.Progress {
			fmt.Fprintf(&sb, "- %s\n", line)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## 结论\n\n")
	sb.WriteString(strings.TrimSpace(msg.Content))
	sb.WriteString("\n")
	return sb.String()
}

// kindLabel 把 OpsDocKind 转成中文短前缀，供标题渲染使用。
func kindLabel(kind core.OpsDocKind) string {
	switch kind {
	case core.OpsDocKindInspection:
		return "巡检"
	case core.OpsDocKindTroubleshooting:
		return "排障"
	case core.OpsDocKindArchitecture:
		return "架构"
	default:
		return "对话"
	}
}
