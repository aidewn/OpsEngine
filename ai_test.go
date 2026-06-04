// AI 工作流生成辅助逻辑测试，不调用外部模型 API。

package main

import (
	"strings"
	"testing"

	"OpsEngine/internal/core"
)

// TestParseGeneratedWorkflow 验证可以从模型回复中提取 JSON 草案。
func TestParseGeneratedWorkflow(t *testing.T) {
	// 模型偶尔会带 Markdown 围栏或前后解释，所以模拟带噪声的输入。
	reply := "好的，这是工作流：\n```json\n{\"name\":\"打印\",\"description\":\"\",\"variables\":[],\"nodes\":[{\"id\":\"n1\",\"type_id\":\"system_ready\",\"config\":{},\"position\":{\"x\":80,\"y\":120}}],\"edges\":[],\"notes\":[]}\n```"
	draft, err := parseGeneratedWorkflow(reply)
	if err != nil {
		t.Fatalf("parseGeneratedWorkflow() error = %v", err)
	}
	if draft.Name != "打印" {
		t.Fatalf("Name = %q", draft.Name)
	}
	if len(draft.Nodes) != 1 || draft.Nodes[0].TypeID != "system_ready" {
		t.Fatalf("Nodes = %#v", draft.Nodes)
	}
}

// TestParseGeneratedWorkflowEmptyNodes 验证空节点列表会被拒绝。
func TestParseGeneratedWorkflowEmptyNodes(t *testing.T) {
	if _, err := parseGeneratedWorkflow(`{"name":"x","nodes":[]}`); err == nil {
		t.Fatal("expected error for empty nodes")
	}
}

// TestMaterializeWorkflow 验证 id 重写、边映射、校验全部走通。
func TestMaterializeWorkflow(t *testing.T) {
	app := &App{}
	draft := aiGeneratedWorkflow{
		Name:        "最小工作流",
		Description: "system_ready -> print",
		Nodes: []aiGeneratedNode{
			{ID: "n1", TypeID: "system_ready", Position: aiPosition{X: 80, Y: 120}},
			{ID: "n2", TypeID: "print", Config: map[string]any{"text": "hello"}, Position: aiPosition{X: 360, Y: 120}},
		},
		Edges: []aiGeneratedEdge{
			{From: aiPortRef{Node: "n1", Port: "exec_out"}, To: aiPortRef{Node: "n2", Port: "exec_in"}},
		},
	}
	workflow, err := app.materializeWorkflow(draft)
	if err != nil {
		t.Fatalf("materializeWorkflow() error = %v", err)
	}
	if len(workflow.Nodes) != 2 || len(workflow.Edges) != 1 {
		t.Fatalf("nodes/edges = %d/%d", len(workflow.Nodes), len(workflow.Edges))
	}
	for _, node := range workflow.Nodes {
		if node.InstanceID == "n1" || node.InstanceID == "n2" {
			t.Fatalf("临时 id 未重写: %s", node.InstanceID)
		}
	}
	// 边引用必须指向重写后的 UUID。
	edge := workflow.Edges[0]
	if edge.From.Node == "n1" || edge.To.Node == "n2" {
		t.Fatalf("边未跟随 id 重写: %#v", edge)
	}
}

// TestMaterializeWorkflowRejectsUnknownType 验证未知节点类型会被拒绝。
func TestMaterializeWorkflowRejectsUnknownType(t *testing.T) {
	app := &App{}
	_, err := app.materializeWorkflow(aiGeneratedWorkflow{
		Nodes: []aiGeneratedNode{{ID: "n1", TypeID: "not_a_real_type"}},
	})
	if err == nil || !strings.Contains(err.Error(), "未知节点类型") {
		t.Fatalf("expected unknown type error, got %v", err)
	}
}

// TestMaterializeWorkflowRejectsDanglingEdge 验证边引用不存在的节点会被拒绝。
func TestMaterializeWorkflowRejectsDanglingEdge(t *testing.T) {
	app := &App{}
	_, err := app.materializeWorkflow(aiGeneratedWorkflow{
		Nodes: []aiGeneratedNode{{ID: "n1", TypeID: "system_ready"}},
		Edges: []aiGeneratedEdge{{From: aiPortRef{Node: "n1", Port: "exec_out"}, To: aiPortRef{Node: "ghost", Port: "exec_in"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "未知节点") {
		t.Fatalf("expected dangling edge error, got %v", err)
	}
}

// TestMakeSessionTitle 验证标题截取和空消息回退。
func TestMakeSessionTitle(t *testing.T) {
	if got := makeSessionTitle("帮我分析下服务器"); got != "帮我分析下服务器" {
		t.Fatalf("短消息原样返回: got %q", got)
	}
	long := strings.Repeat("巡", 50)
	got := makeSessionTitle(long)
	if !strings.HasSuffix(got, "…") || len([]rune(got)) != 31 {
		t.Fatalf("长消息应截到 30 字符 + 省略号: %q (runes=%d)", got, len([]rune(got)))
	}
	if got := makeSessionTitle("   "); got != "新会话" {
		t.Fatalf("空消息回退: got %q", got)
	}
}

// TestIsFirstUserMessage 验证仅当 session 中有且只有一条 user 消息时返回 true。
func TestIsFirstUserMessage(t *testing.T) {
	empty := core.AISession{}
	if isFirstUserMessage(empty) {
		t.Fatal("空 session 不应判定为首条用户消息")
	}
	one := core.AISession{Messages: []core.AISessionMessage{{Role: core.AIMessageRoleUser}}}
	if !isFirstUserMessage(one) {
		t.Fatal("单条 user 消息应判定为首条")
	}
	two := core.AISession{Messages: []core.AISessionMessage{
		{Role: core.AIMessageRoleUser},
		{Role: core.AIMessageRoleAssistant},
		{Role: core.AIMessageRoleUser},
	}}
	if isFirstUserMessage(two) {
		t.Fatal("两条 user 消息不应判定为首条")
	}
}

// TestBuildChatLLMMessagesFiltersEmptyAssistant 验证空 assistant 消息被过滤、
// hidden system 消息仍然进入 LLM 输入。
func TestBuildChatLLMMessagesFiltersEmptyAssistant(t *testing.T) {
	session := core.AISession{
		Messages: []core.AISessionMessage{
			{Role: core.AIMessageRoleSystem, Content: "服务器状态:CPU=20%", Hidden: true},
			{Role: core.AIMessageRoleUser, Content: "分析下"},
			{Role: core.AIMessageRoleAssistant, Content: ""},
			{Role: core.AIMessageRoleUser, Content: "继续"},
		},
	}
	messages := buildChatLLMMessages(session)
	// 基础 system + 预取 system + 两条 user，空 assistant 被过滤。
	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d: %#v", len(messages), messages)
	}
	if messages[1].Role != "system" || !strings.Contains(messages[1].Content, "CPU=20%") {
		t.Fatalf("第二条应为预取上下文: %#v", messages[1])
	}
}

// TestResolveAIAssistantOperation 验证关键词识别工作流意图。
func TestResolveAIAssistantOperation(t *testing.T) {
	cases := []struct {
		name    string
		message string
		want    string
	}{
		{"包含工作流", "帮我生成一份 Linux 服务器巡检工作流", "generate_workflow"},
		{"巡检触发", "做一份服务器巡检", "generate_workflow"},
		{"英文 workflow", "create a deploy workflow", "generate_workflow"},
		{"普通问答", "这台服务器 CPU 持续偏高应该怎么排查", "chat"},
		{"无关闲聊", "你好", "chat"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveAIAssistantOperation("auto", tt.message)
			if got != tt.want {
				t.Fatalf("resolveAIAssistantOperation(%q) = %q, want %q", tt.message, got, tt.want)
			}
		})
	}
}
