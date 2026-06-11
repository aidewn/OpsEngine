// handleExecutionFix 单元测试：验证修复回合的提示词组装与状态门禁。
package runtime

import (
	"strings"
	"testing"
	"time"

	"OpsEngine/internal/core"
)

// failedRecord 构造一条最小失败执行记录。
func failedRecord(status core.WorkflowStatus) core.ExecutionRecord {
	return core.ExecutionRecord{
		ID:         "exec-1",
		WorkflowID: "wf-1",
		Status:     status,
		Snapshot: core.ExecutionSnapshot{
			Workflow: core.WorkflowDef{
				ID: "wf-1", Name: "部署",
				Nodes: []core.NodeInstance{
					{InstanceID: "n1", TypeID: "linux_exec_command", Config: map[string]any{"command": "deploy.sh"}},
				},
			},
		},
		RootFrame: core.FrameState{
			NodeStates: map[string]core.NodeState{"n1": core.NodeStateFailed},
			NodeLogs: map[string][]core.LogEntry{
				"n1": {{Time: time.Now(), Level: "error", Message: "exit_code=127 deploy.sh: not found"}},
			},
		},
	}
}

// fixRuntime 组装一个可跑通修复回合的 Runtime。
func fixRuntime(llm *stubLLM, wf *memWorkflows, rec core.ExecutionRecord) (*Runtime, *memSessions, *bufEmitter) {
	sessions := newMemSessions()
	sessions.data["sess-1"] = core.AISession{ID: "sess-1", Title: "test"}
	emit := &bufEmitter{}
	rt := &Runtime{
		Sessions:  sessions,
		Workflows: wf,
		LLM:       llm,
		Emit:      emit,
		Executions: func(id string) (core.ExecutionRecord, error) {
			return rec, nil
		},
	}
	return rt, sessions, emit
}

// TestHandleExecutionFix 验证完整修复链路：读记录 → 组装失败上下文 → 草案落盘 → 事件与会话回写。
func TestHandleExecutionFix(t *testing.T) {
	existing := core.WorkflowDef{ID: "wf-1", Name: "部署", Nodes: []core.NodeInstance{
		{InstanceID: "n1", TypeID: "linux_exec_command", Config: map[string]any{"command": "deploy.sh"}},
	}}
	wf := &memWorkflows{saved: &existing}
	llm := &stubLLM{reply: `{"name":"部署","nodes":[{"id":"n1","type_id":"linux_exec_command","config":{"command":"/opt/app/deploy.sh"},"position":{"x":0,"y":0}}],"edges":[]}`}
	rt, sessions, emit := fixRuntime(llm, wf, failedRecord(core.WorkflowStatusFailed))

	err := rt.Run(Request{
		RequestID: "req-1", SessionID: "sess-1",
		Operation:   "fix_execution",
		ExecutionID: "exec-1",
		Message:     "请修复这次失败的执行",
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// 提示词应包含结构化失败上下文与当前工作流 JSON。
	if len(llm.gotMessages) == 0 {
		t.Fatal("LLM 未被调用")
	}
	userPrompt := llm.gotMessages[0][1].Content
	for _, want := range []string{"exit_code=127", "linux_exec_command", "当前工作流"} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("修复提示词缺少 %q:\n%s", want, userPrompt)
		}
	}

	// 工作流被更新保存，事件序列含 workflow(update) + done。
	if wf.saved == nil || wf.saved.Nodes[0].Config["command"] != "/opt/app/deploy.sh" {
		t.Fatalf("工作流未按草案更新: %#v", wf.saved)
	}
	var seenUpdate, seenDone bool
	for _, e := range emit.events {
		if e.Type == EventError {
			t.Fatalf("不应出现 error 事件: %#v", emit.events)
		}
		if e.Type == EventWorkflow && e.ActionType == "update" {
			seenUpdate = true
		}
		if e.Type == EventDone {
			seenDone = true
		}
	}
	if !seenUpdate || !seenDone {
		t.Fatalf("缺少 workflow/done 事件: %#v", emit.events)
	}

	// 会话 assistant 消息打上 fix_execution 意图标签。
	final := sessions.data["sess-1"]
	last := final.Messages[len(final.Messages)-1]
	if last.Role != core.AIMessageRoleAssistant || last.Intent != "fix_execution" {
		t.Fatalf("assistant 消息异常: %#v", last)
	}
}

// TestHandleExecutionFixRejectsNonFailed 验证非失败/终止状态的执行被拒绝。
func TestHandleExecutionFixRejectsNonFailed(t *testing.T) {
	existing := core.WorkflowDef{ID: "wf-1", Name: "部署"}
	wf := &memWorkflows{saved: &existing}
	rt, _, emit := fixRuntime(&stubLLM{}, wf, failedRecord(core.WorkflowStatusSuccess))

	if err := rt.Run(Request{
		RequestID: "req-1", SessionID: "sess-1",
		Operation: "fix_execution", ExecutionID: "exec-1", Message: "修复",
	}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	last := emit.events[len(emit.events)-1]
	if last.Type != EventError || !strings.Contains(last.Text, "仅失败/终止") {
		t.Fatalf("应推送状态门禁错误，got %#v", last)
	}
}

// TestHandleExecutionFixMissingID 验证缺少 execution_id 时直接报错。
func TestHandleExecutionFixMissingID(t *testing.T) {
	rt, _, emit := fixRuntime(&stubLLM{}, &memWorkflows{}, failedRecord(core.WorkflowStatusFailed))
	if err := rt.Run(Request{
		RequestID: "req-1", SessionID: "sess-1",
		Operation: "fix_execution", Message: "修复",
	}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	last := emit.events[len(emit.events)-1]
	if last.Type != EventError || !strings.Contains(last.Text, "execution_id") {
		t.Fatalf("应推送缺参错误，got %#v", last)
	}
}
