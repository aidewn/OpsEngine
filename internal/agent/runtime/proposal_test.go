// propose 工具链路测试：脚本化回放 LLM 的工具调用序列，验证生成/更新/配额/预算。
package runtime

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"OpsEngine/internal/agent/tools"
	"OpsEngine/internal/agent/tools/builtin"
	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
)

// scriptedToolLLM 按脚本顺序回放 ChatCompletion，记录每轮收到的 messages。
type scriptedToolLLM struct {
	script      []clients.ChatCompletion
	idx         int
	gotMessages [][]clients.ChatMessage
}

func (s *scriptedToolLLM) Chat(_ []clients.ChatMessage) (string, error) {
	return "", errors.New("scriptedToolLLM 只支持工具循环")
}

func (s *scriptedToolLLM) ChatStream(_ []clients.ChatMessage, _ func(string)) (string, error) {
	return "", errors.New("scriptedToolLLM 只支持工具循环")
}

func (s *scriptedToolLLM) ChatWithTools(messages []clients.ChatMessage, _ []clients.ToolSpec) (clients.ChatCompletion, error) {
	return s.next(messages)
}

func (s *scriptedToolLLM) ChatWithToolsStream(messages []clients.ChatMessage, _ []clients.ToolSpec, onContent func(string)) (clients.ChatCompletion, error) {
	c, err := s.next(messages)
	if err == nil && c.Content != "" && onContent != nil {
		onContent(c.Content)
	}
	return c, err
}

func (s *scriptedToolLLM) next(messages []clients.ChatMessage) (clients.ChatCompletion, error) {
	// 拷贝 messages 供断言（循环内会原地裁剪）
	snapshot := make([]clients.ChatMessage, len(messages))
	copy(snapshot, messages)
	s.gotMessages = append(s.gotMessages, snapshot)
	if s.idx >= len(s.script) {
		return clients.ChatCompletion{}, errors.New("脚本耗尽")
	}
	c := s.script[s.idx]
	s.idx++
	return c, nil
}

// proposeCall 构造一个 propose 工具调用。
func proposeCall(name string, args map[string]any) clients.ToolCall {
	raw, _ := json.Marshal(args)
	return clients.ToolCall{
		ID: "call-1", Type: "function",
		Function: clients.ToolCallFunc{Name: name, Arguments: string(raw)},
	}
}

// validDraftJSON 是最小合法工作流草案。
const validDraftJSON = `{"name":"重启服务","nodes":[{"id":"n1","type_id":"system_ready","config":{},"position":{"x":0,"y":0}}],"edges":[]}`

// newToolLoopRuntime 构造带真实 builtin 注册表的 Runtime。
func newToolLoopRuntime(t *testing.T, llm LLMProvider, wf *memWorkflows) (*Runtime, *memSessions, *bufEmitter) {
	t.Helper()
	reg := tools.NewRegistry()
	if err := builtin.Register(reg, tools.RegistryDeps{}); err != nil {
		t.Fatalf("注册工具失败: %v", err)
	}
	sessions := newMemSessions()
	sessions.data["sess-1"] = core.AISession{ID: "sess-1", Title: "test"}
	emit := &bufEmitter{}
	rt := &Runtime{
		Sessions:  sessions,
		Workflows: wf,
		LLM:       llm,
		Emit:      emit,
		Tools:     reg,
	}
	return rt, sessions, emit
}

// TestGenerateWorkflowViaToolLoop 验证 generate 意图经工具循环 propose 落盘。
func TestGenerateWorkflowViaToolLoop(t *testing.T) {
	llm := &scriptedToolLLM{script: []clients.ChatCompletion{
		{ToolCalls: []clients.ToolCall{proposeCall("propose_workflow", map[string]any{"draft_json": validDraftJSON})}},
		{Content: "已为你创建「重启服务」工作流。"},
	}}
	wf := &memWorkflows{}
	rt, sessions, emit := newToolLoopRuntime(t, llm, wf)

	if err := rt.Run(Request{
		RequestID: "req-1", SessionID: "sess-1",
		Operation: "generate_workflow", Message: "做一个重启服务的工作流",
	}); err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// 系统段应包含编排引导
	var hasGuidance bool
	for _, m := range llm.gotMessages[0] {
		if m.Role == "system" && strings.Contains(m.Content, "工作流编排任务") {
			hasGuidance = true
		}
	}
	if !hasGuidance {
		t.Fatalf("缺少编排引导段: %#v", llm.gotMessages[0])
	}
	// 工作流落盘 + create 事件 + 会话进入编辑模式
	if wf.saved == nil || wf.saved.Name != "重启服务" {
		t.Fatalf("工作流未保存: %#v", wf.saved)
	}
	var seenCreate, seenDone bool
	for _, e := range emit.events {
		if e.Type == EventWorkflow && e.ActionType == "create" {
			seenCreate = true
		}
		if e.Type == EventDone {
			seenDone = true
		}
		if e.Type == EventError {
			t.Fatalf("不应出现 error: %#v", emit.events)
		}
	}
	if !seenCreate || !seenDone {
		t.Fatalf("事件缺失: %#v", emit.events)
	}
	final := sessions.data["sess-1"]
	if final.ActiveArtifactID != wf.saved.ID {
		t.Fatalf("会话应进入编辑模式: %#v", final)
	}
	last := final.Messages[len(final.Messages)-1]
	if last.Intent != "generate_workflow" || !strings.Contains(last.Content, "重启服务") {
		t.Fatalf("assistant 消息异常: %#v", last)
	}
}

// TestProposeValidationFeedback 验证校验失败的错误经 tool 消息反馈，模型修正后成功。
func TestProposeValidationFeedback(t *testing.T) {
	badDraft := `{"name":"坏","nodes":[{"id":"n1","type_id":"no_such_type","config":{},"position":{"x":0,"y":0}}],"edges":[]}`
	llm := &scriptedToolLLM{script: []clients.ChatCompletion{
		{ToolCalls: []clients.ToolCall{proposeCall("propose_workflow", map[string]any{"draft_json": badDraft})}},
		{ToolCalls: []clients.ToolCall{proposeCall("propose_workflow", map[string]any{"draft_json": validDraftJSON})}},
		{Content: "完成。"},
	}}
	wf := &memWorkflows{}
	rt, _, _ := newToolLoopRuntime(t, llm, wf)
	rt.NodeChecker = func(typeID string) error {
		if typeID == "no_such_type" {
			return errors.New("未知节点类型: no_such_type")
		}
		return nil
	}

	if err := rt.Run(Request{
		RequestID: "req-1", SessionID: "sess-1",
		Operation: "generate_workflow", Message: "生成",
	}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// 第二轮 LLM 输入应含校验错误的 tool 消息
	var sawError bool
	for _, m := range llm.gotMessages[1] {
		if m.Role == "tool" && strings.Contains(m.Content, "no_such_type") {
			sawError = true
		}
	}
	if !sawError {
		t.Fatalf("校验错误未反馈给模型: %#v", llm.gotMessages[1])
	}
	if wf.saved == nil || wf.saved.Name != "重启服务" {
		t.Fatalf("修正后的工作流未保存: %#v", wf.saved)
	}
}

// TestProposeUpdateConfirmMode 验证更新意图在确认模式下经工具循环进入待确认草案。
func TestProposeUpdateConfirmMode(t *testing.T) {
	existing := core.WorkflowDef{ID: "wf-1", Name: "部署", Nodes: []core.NodeInstance{
		{InstanceID: "keep-1", TypeID: "system_ready", Config: map[string]any{}},
	}}
	updateDraft := `{"name":"部署","nodes":[{"id":"keep-1","type_id":"system_ready","config":{},"position":{"x":0,"y":0}},{"id":"n2","type_id":"print","config":{"message":"done"},"position":{"x":0,"y":0}}],"edges":[]}`
	llm := &scriptedToolLLM{script: []clients.ChatCompletion{
		{ToolCalls: []clients.ToolCall{proposeCall("propose_update_workflow", map[string]any{
			"workflow_id": "wf-1", "draft_json": updateDraft,
		})}},
		{Content: "草案已生成，请确认。"},
	}}
	wf := &memWorkflows{saved: &existing}
	rt, sessions, emit := newToolLoopRuntime(t, llm, wf)
	rt.ApplyMode = ApplyModeConfirm

	if err := rt.Run(Request{
		RequestID: "req-1", SessionID: "sess-1",
		Operation: "update_workflow", ArtifactID: "wf-1", Message: "结尾加一个打印",
	}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// 不落盘，草案在会话上，节点身份保留
	if len(wf.saved.Nodes) != 1 {
		t.Fatalf("确认模式不应直接落盘: %#v", wf.saved)
	}
	pending := sessions.data["sess-1"].PendingDraft
	if pending == nil || pending.ArtifactID != "wf-1" {
		t.Fatalf("草案缺失: %#v", pending)
	}
	if !strings.Contains(pending.ChangeSummary, "新增 1 节点") {
		t.Fatalf("diff 摘要异常: %s", pending.ChangeSummary)
	}
	var seenPending bool
	for _, e := range emit.events {
		if e.Type == EventWorkflowPending {
			seenPending = true
		}
	}
	if !seenPending {
		t.Fatalf("缺少 workflow_pending 事件: %#v", emit.events)
	}
	// 应用草案后落盘
	applied, err := rt.ApplyPendingDraft("sess-1")
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if len(applied.Nodes) != 2 || applied.Nodes[0].InstanceID != "keep-1" {
		t.Fatalf("应用结果异常: %#v", applied.Nodes)
	}
}

// TestProposalQuota 验证单轮成功提交超过上限被拒绝。
func TestProposalQuota(t *testing.T) {
	calls := []clients.ChatCompletion{}
	for i := 0; i < maxProposalsPerTurn+1; i++ {
		calls = append(calls, clients.ChatCompletion{
			ToolCalls: []clients.ToolCall{proposeCall("propose_workflow", map[string]any{"draft_json": validDraftJSON})},
		})
	}
	calls = append(calls, clients.ChatCompletion{Content: "结束。"})
	llm := &scriptedToolLLM{script: calls}
	wf := &memWorkflows{}
	rt, _, _ := newToolLoopRuntime(t, llm, wf)

	if err := rt.Run(Request{
		RequestID: "req-1", SessionID: "sess-1",
		Operation: "generate_workflow", Message: "生成",
	}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// 第 3 次提交应收到配额错误
	lastRound := llm.gotMessages[len(llm.gotMessages)-1]
	var sawQuota bool
	for _, m := range lastRound {
		if m.Role == "tool" && strings.Contains(m.Content, "最多提交") {
			sawQuota = true
		}
	}
	if !sawQuota {
		t.Fatalf("配额限制未生效: %#v", lastRound)
	}
}

// TestPruneToolMessages 验证上下文预算从最旧 tool 消息开始裁剪。
func TestPruneToolMessages(t *testing.T) {
	big := strings.Repeat("x", 500)
	messages := []clients.ChatMessage{
		{Role: "system", Content: big},
		{Role: "user", Content: "问题"},
		{Role: "tool", Content: big},
		{Role: "tool", Content: big},
		{Role: "assistant", Content: "推理"},
		{Role: "tool", Content: big},
	}
	pruneToolMessages(messages, 1200)
	if messages[2].Content != prunedPlaceholder || messages[3].Content != prunedPlaceholder {
		t.Fatalf("最旧的 tool 消息应被裁剪: %q %q", messages[2].Content[:20], messages[3].Content[:20])
	}
	if messages[5].Content != big {
		t.Fatal("预算满足后不应继续裁剪")
	}
	if messages[0].Content != big || messages[4].Content != "推理" {
		t.Fatal("system/assistant 消息不应被裁剪")
	}
}
