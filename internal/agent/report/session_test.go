// FromSessionMessage 单测：标题渲染、关键段落齐全、空内容拒绝。

package report

import (
	"strings"
	"testing"
	"time"

	"OpsEngine/internal/core"
)

func sampleSession() (core.EnvironmentDef, core.AISession, core.AISessionMessage) {
	env := core.EnvironmentDef{ID: "env-1", Name: "生产环境"}
	session := core.AISession{ID: "sess-1", Title: "排查 CPU 高", EnvironmentID: "env-1"}
	msg := core.AISessionMessage{
		ID: "msg-1", Role: core.AIMessageRoleAssistant,
		Content:   "## 事实\n- CPU 95%\n## 判断\n- 很可能是 java 进程\n## 建议\n- 看 stacktrace",
		Progress:  []string{"🔧 调用 ssh_process_list()", "✓ ssh_process_list top 10"},
		Intent:    "troubleshoot",
		CreatedAt: time.Date(2026, 6, 10, 14, 30, 0, 0, time.UTC),
	}
	return env, session, msg
}

func TestFromSessionMessageIncludesAllSections(t *testing.T) {
	env, session, msg := sampleSession()
	doc, err := FromSessionMessage(env, session, msg, core.OpsDocKindTroubleshooting)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if doc.Kind != core.OpsDocKindTroubleshooting {
		t.Fatalf("kind = %q", doc.Kind)
	}
	for _, want := range []string{
		"# 排障报告：排查 CPU 高",
		"## 来源",
		"环境：生产环境",
		"sess-1",
		"## 采集过程",
		"🔧 调用 ssh_process_list",
		"## 结论",
		"## 事实", "## 判断", "## 建议",
	} {
		if !strings.Contains(doc.Body, want) {
			t.Fatalf("缺少片段 %q\n%s", want, doc.Body)
		}
	}
	if doc.Source.SessionID != "sess-1" || doc.Source.EnvironmentID != "env-1" {
		t.Fatalf("source 未带入: %#v", doc.Source)
	}
}

func TestFromSessionMessageRejectsEmpty(t *testing.T) {
	env, session, msg := sampleSession()
	msg.Content = "   "
	if _, err := FromSessionMessage(env, session, msg, core.OpsDocKindTroubleshooting); err == nil {
		t.Fatal("空正文应被拒绝")
	}
}

func TestFromSessionMessageRejectsNonAssistant(t *testing.T) {
	env, session, msg := sampleSession()
	msg.Role = core.AIMessageRoleUser
	if _, err := FromSessionMessage(env, session, msg, core.OpsDocKindTroubleshooting); err == nil {
		t.Fatal("非 assistant 消息应被拒绝")
	}
}
