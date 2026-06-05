// Prompt Builder 的最小验证测试：模板渲染走通、关键约束写进 prompt、空 assistant 被过滤。

package prompt

import (
	"strings"
	"testing"

	"OpsEngine/internal/core"
)

// TestBuildWorkflowSystemPrompt 验证模板渲染包含节点目录、环境列表、用户偏好三段输入。
func TestBuildWorkflowSystemPrompt(t *testing.T) {
	out, err := BuildWorkflowSystemPrompt(WorkflowInputs{
		NodeTypes: []core.NodeTypeDef{{TypeID: "system_ready", DisplayName: "入口"}},
		Environments: []core.EnvironmentDef{{
			ID: "env-1", Name: "测试环境",
			Configs: []core.EnvConfigItem{{ID: "cfg-1", Name: "linux", Kind: core.EnvConfigKindSSH}},
		}},
		PreferredEnvironmentID: "env-1",
		PreferredConfigID:      "cfg-1",
	})
	if err != nil {
		t.Fatalf("BuildWorkflowSystemPrompt error: %v", err)
	}
	for _, want := range []string{
		"system_ready", "env-1", "cfg-1", "硬约束", "只允许返回单个 JSON",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("prompt 缺少关键片段 %q\nprompt=%s", want, out)
		}
	}
}

// TestBuildChatMessagesFiltersEmptyAssistant 验证空 assistant 消息被过滤，
// hidden system 消息仍然进入 LLM 输入。
func TestBuildChatMessagesFiltersEmptyAssistant(t *testing.T) {
	session := core.AISession{
		Messages: []core.AISessionMessage{
			{Role: core.AIMessageRoleSystem, Content: "服务器状态:CPU=20%", Hidden: true},
			{Role: core.AIMessageRoleUser, Content: "分析下"},
			{Role: core.AIMessageRoleAssistant, Content: ""},
			{Role: core.AIMessageRoleUser, Content: "继续"},
		},
	}
	messages, err := BuildChatMessages(session.Messages, ChatContext{})
	if err != nil {
		t.Fatalf("BuildChatMessages error: %v", err)
	}
	// 基础 system + 预取 system + 两条 user，空 assistant 被过滤。
	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d: %#v", len(messages), messages)
	}
	if messages[0].Role != "system" || messages[0].Content == "" {
		t.Fatalf("第一条应为基础 system prompt: %#v", messages[0])
	}
	if messages[1].Role != "system" || !strings.Contains(messages[1].Content, "CPU=20%") {
		t.Fatalf("第二条应为预取上下文: %#v", messages[1])
	}
}
