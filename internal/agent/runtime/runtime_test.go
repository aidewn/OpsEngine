// Runtime 集成测试：用内存 stub 验证 Run 调度与 inspection 路径的完整事件序列。

package runtime

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"

	_ "OpsEngine/internal/nodes" // 触发节点注册，让 engine.ValidateWorkflow 在 inspection.Materialize 中可用
)

// memSessions 是 SessionStore 的内存实现，按 id 索引。
type memSessions struct {
	mu   sync.Mutex
	data map[string]core.AISession
}

func newMemSessions() *memSessions { return &memSessions{data: map[string]core.AISession{}} }

func (m *memSessions) Get(id string) (core.AISession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.data[id]
	if !ok {
		return core.AISession{}, errors.New("session 不存在")
	}
	return s, nil
}

func (m *memSessions) Save(s core.AISession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[s.ID] = s
	return nil
}

// memWorkflows 是 WorkflowSaver 的内存实现，仅记录最近一次写入。
type memWorkflows struct{ saved *core.WorkflowDef }

func (m *memWorkflows) Get(id string) (core.WorkflowDef, error) {
	if m.saved != nil && m.saved.ID == id {
		return *m.saved, nil
	}
	return core.WorkflowDef{}, errors.New("workflow 不存在")
}

func (m *memWorkflows) Save(wf core.WorkflowDef) error {
	wf2 := wf
	m.saved = &wf2
	return nil
}

// memAssembles 是 AssembleSaver 的内存实现，仅记录最近一次写入。
type memAssembles struct{ saved *core.AssembleDef }

func (m *memAssembles) Get(id string) (core.AssembleDef, error) {
	if m.saved != nil && m.saved.ID == id {
		return *m.saved, nil
	}
	return core.AssembleDef{}, errors.New("assemble 不存在")
}

func (m *memAssembles) Save(asm core.AssembleDef) error {
	asm2 := asm
	m.saved = &asm2
	return nil
}

// stubLLM 是固定回复的 LLMProvider，记录收到的 messages 方便断言。
type stubLLM struct {
	reply        string
	streamChunks []string
	gotMessages  [][]clients.ChatMessage
}

func (s *stubLLM) Chat(messages []clients.ChatMessage) (string, error) {
	s.gotMessages = append(s.gotMessages, messages)
	return s.reply, nil
}

func (s *stubLLM) ChatStream(messages []clients.ChatMessage, onDelta func(string)) (string, error) {
	s.gotMessages = append(s.gotMessages, messages)
	for _, c := range s.streamChunks {
		onDelta(c)
	}
	return strings.Join(s.streamChunks, ""), nil
}

// ChatWithTools 在测试 stub 中等价于 Chat，但返回 ChatCompletion 结构。
func (s *stubLLM) ChatWithTools(messages []clients.ChatMessage, _ []clients.ToolSpec) (clients.ChatCompletion, error) {
	s.gotMessages = append(s.gotMessages, messages)
	return clients.ChatCompletion{Content: s.reply}, nil
}

// ChatWithToolsStream 把 reply 整段当作单次 delta 推回，足够单测使用。
func (s *stubLLM) ChatWithToolsStream(messages []clients.ChatMessage, _ []clients.ToolSpec, onContent func(string)) (clients.ChatCompletion, error) {
	s.gotMessages = append(s.gotMessages, messages)
	if s.reply != "" && onContent != nil {
		onContent(s.reply)
	}
	return clients.ChatCompletion{Content: s.reply}, nil
}

// bufEmitter 把事件累计到 slice，供测试断言。
type bufEmitter struct {
	mu     sync.Mutex
	events []Event
}

func (b *bufEmitter) Emit(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, e)
}

func (b *bufEmitter) types() []EventType {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]EventType, len(b.events))
	for i, e := range b.events {
		out[i] = e.Type
	}
	return out
}

// TestRunInspectionHappyPath 端到端验证：用户输入巡检请求 → Runtime 调用 LLM 拿 Plan → 落地工作流 → 推 done 事件 + 写回会话。
func TestRunInspectionHappyPath(t *testing.T) {
	sessions := newMemSessions()
	sessions.data["sess-1"] = core.AISession{
		ID: "sess-1", Title: "test",
		EnvironmentID: "env-1", Scope: core.AISessionScopeConfig, ConfigID: "ssh-1",
	}
	llm := &stubLLM{reply: `{"name":"巡检","items":[{"title":"系统","command":"uname -a"},{"title":"磁盘","command":"df -h"}]}`}
	emit := &bufEmitter{}
	wf := &memWorkflows{}

	envs := map[string]core.EnvironmentDef{
		"env-1": {ID: "env-1", Name: "test-env", Configs: []core.EnvConfigItem{
			{ID: "ssh-1", Name: "host", Kind: core.EnvConfigKindSSH},
		}},
	}
	rt := &Runtime{
		Sessions:  sessions,
		Workflows: wf,
		LLM:       llm,
		Emit:      emit,
		Environments: func(id string) (core.EnvironmentDef, error) {
			env, ok := envs[id]
			if !ok {
				return core.EnvironmentDef{}, errors.New("env not found")
			}
			return env, nil
		},
	}
	if err := rt.Run(Request{RequestID: "req-1", SessionID: "sess-1", Message: "帮我做服务器巡检"}); err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// 事件序列应包含若干 progress + 1 个 workflow + 1 个 done，无 error。
	gotTypes := emit.types()
	var seenWorkflow, seenDone bool
	for _, et := range gotTypes {
		switch et {
		case EventError:
			t.Fatalf("不应出现 error 事件: %#v", emit.events)
		case EventWorkflow:
			seenWorkflow = true
		case EventDone:
			seenDone = true
		}
	}
	if !seenWorkflow || !seenDone {
		t.Fatalf("缺少 workflow/done 事件: %v", gotTypes)
	}

	// 工作流被写入：system_ready + env_connect_ssh + 2 个 linux_exec_command。
	if wf.saved == nil || len(wf.saved.Nodes) != 4 {
		t.Fatalf("工作流未正确保存: %#v", wf.saved)
	}

	// 会话被持久化两次：追加 user 消息一次，追加 assistant 消息一次。
	final := sessions.data["sess-1"]
	if len(final.Messages) != 2 {
		t.Fatalf("session messages = %d, want 2", len(final.Messages))
	}
	if final.Messages[0].Role != core.AIMessageRoleUser || final.Messages[1].Role != core.AIMessageRoleAssistant {
		t.Fatalf("消息角色顺序异常: %#v", final.Messages)
	}
	if final.Messages[1].WorkflowID == "" {
		t.Fatal("assistant 消息应带 WorkflowID 供前端跳转")
	}
}

// TestRunInspectionTargetSelect 验证环境级会话遇到多个 SSH 时发 target_select 事件供前端追问。
func TestRunInspectionTargetSelect(t *testing.T) {
	sessions := newMemSessions()
	sessions.data["sess-1"] = core.AISession{
		ID: "sess-1", Title: "test", EnvironmentID: "env-1", Scope: core.AISessionScopeEnvironment,
	}
	emit := &bufEmitter{}
	rt := &Runtime{
		Sessions:  sessions,
		Workflows: &memWorkflows{},
		LLM:       &stubLLM{reply: `{"name":"巡检","items":[{"title":"系统","command":"uname -a"}]}`},
		Emit:      emit,
		Environments: func(id string) (core.EnvironmentDef, error) {
			return core.EnvironmentDef{ID: id, Configs: []core.EnvConfigItem{
				{ID: "ssh-a", Name: "web", Kind: core.EnvConfigKindSSH},
				{ID: "ssh-b", Name: "db", Kind: core.EnvConfigKindSSH},
			}}, nil
		},
	}
	if err := rt.Run(Request{RequestID: "req-1", SessionID: "sess-1", Message: "帮我巡检"}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	last := emit.events[len(emit.events)-1]
	if last.Type != EventTargetSelect {
		t.Fatalf("expected target_select, got %#v", emit.events)
	}
	if len(last.TargetOptions) != 2 || last.TargetOptions[0].ID != "ssh-a" || last.TargetOptions[1].ID != "ssh-b" {
		t.Fatalf("候选 SSH 异常: %#v", last.TargetOptions)
	}
}

// TestRunInspectionTargetResumeNoDuplicateUser 验证用户选择 SSH 后继续执行，不重复追加同一条 user 消息。
func TestRunInspectionTargetResumeNoDuplicateUser(t *testing.T) {
	sessions := newMemSessions()
	sessions.data["sess-1"] = core.AISession{
		ID: "sess-1", Title: "test", EnvironmentID: "env-1", Scope: core.AISessionScopeEnvironment,
		Messages: []core.AISessionMessage{{
			ID: "u-1", Role: core.AIMessageRoleUser, Content: "帮我巡检",
		}},
	}
	emit := &bufEmitter{}
	wf := &memWorkflows{}
	rt := &Runtime{
		Sessions:  sessions,
		Workflows: wf,
		LLM:       &stubLLM{reply: `{"name":"巡检","items":[{"title":"系统","command":"uname -a"}]}`},
		Emit:      emit,
		Environments: func(id string) (core.EnvironmentDef, error) {
			return core.EnvironmentDef{ID: id, Configs: []core.EnvConfigItem{
				{ID: "ssh-a", Name: "web", Kind: core.EnvConfigKindSSH},
				{ID: "ssh-b", Name: "db", Kind: core.EnvConfigKindSSH},
			}}, nil
		},
	}
	if err := rt.Run(Request{
		RequestID: "req-1", SessionID: "sess-1", Message: "帮我巡检", TargetConfigID: "ssh-b",
	}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if wf.saved == nil {
		t.Fatal("选择目标后应保存巡检工作流")
	}
	final := sessions.data["sess-1"]
	userCount := 0
	for _, m := range final.Messages {
		if m.Role == core.AIMessageRoleUser {
			userCount++
		}
	}
	if userCount != 1 {
		t.Fatalf("user 消息重复追加，count=%d messages=%#v", userCount, final.Messages)
	}
}

// TestRunCreateAssembleGeneralSession 验证通用会话无需环境即可生成集合资产。
func TestRunCreateAssembleGeneralSession(t *testing.T) {
	sessions := newMemSessions()
	sessions.data["sess-1"] = core.AISession{
		ID: "sess-1", Title: "general", Scope: core.AISessionScopeGeneral,
	}
	emit := &bufEmitter{}
	asmStore := &memAssembles{}
	rt := &Runtime{
		Sessions:  sessions,
		Assembles: asmStore,
		LLM: &stubLLM{reply: `{
			"name":"安装 Docker",
			"description":"安装 Docker 和 Docker Compose",
			"params":[],
			"returns":[],
			"variables":[],
			"nodes":[
				{"id":"n1","type_id":"assemble_start","config":{},"position":{"x":80,"y":120}},
				{"id":"n2","type_id":"assemble_end","config":{},"position":{"x":360,"y":120}}
			],
			"edges":[{"from":{"node":"n1","port":"exec_out"},"to":{"node":"n2","port":"exec_in"}}],
			"notes":[]
		}`},
		Emit: emit,
	}
	if err := rt.Run(Request{RequestID: "req-1", SessionID: "sess-1", Operation: "create_assemble", Message: "生成安装 Docker 和 Docker Compose 的集合"}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if asmStore.saved == nil || asmStore.saved.Name != "安装 Docker" {
		t.Fatalf("集合未保存: %#v", asmStore.saved)
	}
	var seenAssemble bool
	for _, event := range emit.events {
		if event.Type == EventAssemble && event.AssembleID != "" {
			seenAssemble = true
		}
	}
	if !seenAssemble {
		t.Fatalf("缺少 assemble 事件: %#v", emit.events)
	}
	final := sessions.data["sess-1"]
	if len(final.Messages) != 2 || final.Messages[1].AssembleID == "" {
		t.Fatalf("会话未记录集合产物: %#v", final.Messages)
	}
}

// TestRunRejectsEmptySession 验证缺 session_id 时通过事件报错而不是返回 error。
func TestRunRejectsEmptySession(t *testing.T) {
	emit := &bufEmitter{}
	rt := &Runtime{Emit: emit, Sessions: newMemSessions()}
	if err := rt.Run(Request{RequestID: "req-1", Message: "hi"}); err != nil {
		t.Fatalf("Run 不应返回 err，应通过事件上报: %v", err)
	}
	if len(emit.events) == 0 || emit.events[0].Type != EventError {
		t.Fatalf("expected EventError, got %#v", emit.events)
	}
}

// TestRunRejectsMissingRequestID 验证 request_id 缺失走返回值通道（无法通过事件归属）。
func TestRunRejectsMissingRequestID(t *testing.T) {
	rt := &Runtime{Emit: &bufEmitter{}, Sessions: newMemSessions()}
	err := rt.Run(Request{SessionID: "s", Message: "hi"})
	if err == nil {
		t.Fatal("missing request_id 应返回 error")
	}
}

// TestMakeSessionTitle 沿用旧 ai_test.go 中的边界用例，确保搬入 runtime 后行为不变。
func TestMakeSessionTitle(t *testing.T) {
	if got := makeSessionTitle("帮我分析下服务器"); got != "帮我分析下服务器" {
		t.Fatalf("短消息原样返回: %q", got)
	}
	long := strings.Repeat("巡", 50)
	got := makeSessionTitle(long)
	if !strings.HasSuffix(got, "…") || len([]rune(got)) != 31 {
		t.Fatalf("长消息截 30 字符 + 省略号: %q", got)
	}
	if got := makeSessionTitle("   "); got != "新会话" {
		t.Fatalf("空消息回退: %q", got)
	}
}
