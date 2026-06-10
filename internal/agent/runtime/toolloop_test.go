// 工具循环测试：用 stub LLM 模拟一次工具调用 + 最终文本回复，验证循环正确收敛。

package runtime

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"OpsEngine/internal/agent/tools"
	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
)

// scriptedLLM 是 LLMProvider 的 stub：按 turns 顺序逐轮返回 ChatCompletion。
// 第 N 轮 ChatWithTools 调用返回 turns[N]。如超出长度则返回最后一个。
type scriptedLLM struct {
	mu        sync.Mutex
	turns     []clients.ChatCompletion
	idx       int
	seenCalls [][]clients.ChatMessage
}

func (s *scriptedLLM) Chat([]clients.ChatMessage) (string, error)                  { return "", nil }
func (s *scriptedLLM) ChatStream([]clients.ChatMessage, func(string)) (string, error) { return "", nil }
func (s *scriptedLLM) ChatWithTools(messages []clients.ChatMessage, _ []clients.ToolSpec) (clients.ChatCompletion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seenCalls = append(s.seenCalls, messages)
	i := s.idx
	if i >= len(s.turns) {
		i = len(s.turns) - 1
	}
	s.idx++
	return s.turns[i], nil
}

// ChatWithToolsStream 把当前 turn 的 content 分两段回推，模拟真实流式行为。
// 没有内容则不回推（保持 tool-only turn 的语义）。
func (s *scriptedLLM) ChatWithToolsStream(messages []clients.ChatMessage, _ []clients.ToolSpec, onContent func(string)) (clients.ChatCompletion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seenCalls = append(s.seenCalls, messages)
	i := s.idx
	if i >= len(s.turns) {
		i = len(s.turns) - 1
	}
	s.idx++
	turn := s.turns[i]
	// 把整段 content 切成两段推回，验证调用方按顺序累积
	if turn.Content != "" && onContent != nil {
		half := len(turn.Content) / 2
		onContent(turn.Content[:half])
		onContent(turn.Content[half:])
	}
	return turn, nil
}

// echoTool 返回固定文本，记录被调用次数。
type echoTool struct {
	called int
}

func (e *echoTool) Spec() tools.Spec {
	return tools.Spec{
		Name: "echo", Description: "echoes hello", Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{},
	}
}
func (e *echoTool) Execute(_ tools.ToolContext, _ map[string]any) (tools.Result, error) {
	e.called++
	return tools.Result{Output: "hello world", DisplaySummary: "echo"}, nil
}

// TestToolLoopStreamsFinalContentToUI 验证最终回复 content 在流式过程中
// 通过 EventDelta 推送到 UI，而不是循环结束后一次性 emit。
// 这是 chat 工具循环用户体验改进的核心验证点。
func TestToolLoopStreamsFinalContentToUI(t *testing.T) {
	reg := tools.NewRegistry()
	if err := reg.Register(&echoTool{}); err != nil {
		t.Fatal(err)
	}
	llm := &scriptedLLM{turns: []clients.ChatCompletion{
		// 第一轮纯工具调用，无 content
		{ToolCalls: []clients.ToolCall{{
			ID: "c1", Type: "function",
			Function: clients.ToolCallFunc{Name: "echo", Arguments: "{}"},
		}}},
		// 第二轮纯文本最终回复——scriptedLLM 会把它切成两段流式推
		{Content: "已完成分析。"},
	}}
	sessions := newMemSessions()
	sessions.data["s"] = core.AISession{ID: "s", EnvironmentID: "e"}
	emit := &bufEmitter{}
	rt := &Runtime{
		Sessions: sessions, Emit: emit, LLM: llm, Tools: reg,
		Environments: func(string) (core.EnvironmentDef, error) {
			return core.EnvironmentDef{ID: "e"}, nil
		},
	}
	if err := rt.Run(Request{RequestID: "r", SessionID: "s", Message: "做点事"}); err != nil {
		t.Fatal(err)
	}
	// 期望至少 2 个 EventDelta（来自 scriptedLLM 把 content 切两段）
	deltas := []string{}
	for _, e := range emit.events {
		if e.Type == EventDelta {
			deltas = append(deltas, e.Text)
		}
	}
	if len(deltas) < 2 {
		t.Fatalf("最终 content 应该被切成多段 delta 推送，实际只收到 %d 段: %#v", len(deltas), deltas)
	}
	// 拼接后应等于完整文本
	if got := strings.Join(deltas, ""); got != "已完成分析。" {
		t.Fatalf("delta 拼接结果与原文不一致: %q", got)
	}
}

// TestToolLoopExecutesAndConverges 验证 一次工具调用 → 一次最终文本 这条主路径。
func TestToolLoopExecutesAndConverges(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &echoTool{}
	if err := reg.Register(tool); err != nil {
		t.Fatal(err)
	}

	llm := &scriptedLLM{turns: []clients.ChatCompletion{
		// 第一轮：模型请求调用 echo
		{ToolCalls: []clients.ToolCall{{
			ID: "call_1", Type: "function",
			Function: clients.ToolCallFunc{Name: "echo", Arguments: "{}"},
		}}},
		// 第二轮：模型基于工具结果产出最终文本
		{Content: "工具返回了 hello world，已分析完成。"},
	}}

	sessions := newMemSessions()
	sessions.data["sess-1"] = core.AISession{ID: "sess-1", EnvironmentID: "env-1"}
	emit := &bufEmitter{}
	rt := &Runtime{
		Sessions: sessions, Emit: emit, LLM: llm, Tools: reg,
		Environments: func(string) (core.EnvironmentDef, error) {
			return core.EnvironmentDef{ID: "env-1", Name: "test"}, nil
		},
	}
	if err := rt.Run(Request{RequestID: "r1", SessionID: "sess-1", Message: "你好"}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if tool.called != 1 {
		t.Fatalf("expected echo called once, got %d", tool.called)
	}
	if llm.idx != 2 {
		t.Fatalf("expected 2 LLM rounds, got %d", llm.idx)
	}

	// 最终消息内容应来自第二轮
	final := sessions.data["sess-1"]
	last := final.Messages[len(final.Messages)-1]
	if last.Role != core.AIMessageRoleAssistant || !strings.Contains(last.Content, "hello world") {
		t.Fatalf("最终消息内容异常: %#v", last)
	}
	// 工具调用进度应有 🔧 起的两条（调用 + 完成）
	gotProgress := strings.Join(last.Progress, "|")
	if !strings.Contains(gotProgress, "🔧 调用 echo") || !strings.Contains(gotProgress, "✓ echo") {
		t.Fatalf("progress 缺少工具痕迹: %s", gotProgress)
	}
}

// TestToolLoopMaxRoundsGuard 验证防御性上限：模型一直请求工具时不会死循环。
func TestToolLoopMaxRoundsGuard(t *testing.T) {
	reg := tools.NewRegistry()
	if err := reg.Register(&echoTool{}); err != nil {
		t.Fatal(err)
	}
	llm := &scriptedLLM{turns: []clients.ChatCompletion{
		{ToolCalls: []clients.ToolCall{{
			ID: "call", Type: "function",
			Function: clients.ToolCallFunc{Name: "echo", Arguments: "{}"},
		}}},
	}}
	sessions := newMemSessions()
	sessions.data["s"] = core.AISession{ID: "s", EnvironmentID: "e"}
	emit := &bufEmitter{}
	rt := &Runtime{
		Sessions: sessions, Emit: emit, LLM: llm, Tools: reg,
		MaxToolRounds: 2,
		Environments: func(string) (core.EnvironmentDef, error) {
			return core.EnvironmentDef{ID: "e"}, nil
		},
	}
	if err := rt.Run(Request{RequestID: "r", SessionID: "s", Message: "x"}); err != nil {
		t.Fatal(err)
	}
	// 应该在第 2 轮后给出 error 事件，而不是无限增长
	var sawError bool
	for _, e := range emit.events {
		if e.Type == EventError {
			sawError = true
		}
	}
	if !sawError {
		t.Fatal("超出 MaxToolRounds 应触发 EventError")
	}
}

// TestToolLoopUnknownToolReportsErrorButContinues 验证未注册工具不会让循环崩，
// 而是把"未注册的工具"作为 tool 消息回灌给模型，让模型自己处理。
func TestToolLoopUnknownToolReportsErrorButContinues(t *testing.T) {
	reg := tools.NewRegistry()
	// 不注册任何工具
	llm := &scriptedLLM{turns: []clients.ChatCompletion{
		{ToolCalls: []clients.ToolCall{{
			ID: "c1", Type: "function",
			Function: clients.ToolCallFunc{Name: "ghost", Arguments: "{}"},
		}}},
		{Content: "工具不存在，我直接回答。"},
	}}
	sessions := newMemSessions()
	sessions.data["s"] = core.AISession{ID: "s", EnvironmentID: "e"}
	emit := &bufEmitter{}

	// 必须给 Registry 至少一个工具，否则 chat 走流式路径不会触发循环
	reg2 := tools.NewRegistry()
	_ = reg2.Register(&echoTool{})
	rt := &Runtime{
		Sessions: sessions, Emit: emit, LLM: llm, Tools: reg2,
		Environments: func(string) (core.EnvironmentDef, error) {
			return core.EnvironmentDef{ID: "e"}, nil
		},
	}
	if err := rt.Run(Request{RequestID: "r", SessionID: "s", Message: "x"}); err != nil {
		t.Fatal(err)
	}
	_ = reg // 占位防止未使用警告
	// 应该有错误进度（未注册工具）+ 最终消息
	final := sessions.data["s"]
	if final.Messages[len(final.Messages)-1].Role != core.AIMessageRoleAssistant {
		t.Fatal("应有最终 assistant 消息")
	}
	var sawUnknownProgress bool
	for _, e := range emit.events {
		if e.Type == EventProgress && strings.Contains(e.Text, "未注册的工具") {
			sawUnknownProgress = true
		}
	}
	if !sawUnknownProgress {
		t.Fatal("应在 progress 中提示未注册工具")
	}
}

// 确保 errors 包导入被使用（针对未来 IDE 自动清理可能误删 import）。
var _ = errors.New
