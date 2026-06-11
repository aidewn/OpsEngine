// 确认模式（pending draft）单元测试：暂存、应用、基线漂移、放弃。
package runtime

import (
	"strings"
	"testing"

	"OpsEngine/internal/core"
)

// stagedRuntime 跑一轮确认模式下的 fix_execution，返回已暂存草案的运行环境。
func stagedRuntime(t *testing.T) (*Runtime, *memSessions, *memWorkflows, *bufEmitter) {
	t.Helper()
	existing := core.WorkflowDef{ID: "wf-1", Name: "部署", Nodes: []core.NodeInstance{
		{InstanceID: "n1", TypeID: "linux_exec_command", Config: map[string]any{"command": "deploy.sh"}},
	}}
	wf := &memWorkflows{saved: &existing}
	llm := &stubLLM{reply: `{"name":"部署","nodes":[{"id":"r1","type_id":"system_ready","config":{},"position":{"x":0,"y":0}},{"id":"n1","type_id":"linux_exec_command","config":{"command":"/opt/app/deploy.sh"},"position":{"x":0,"y":0}}],"edges":[{"from":{"node":"r1","port":"exec_out"},"to":{"node":"n1","port":"exec_in"}}]}`}
	rt, sessions, emit := fixRuntime(llm, wf, failedRecord(core.WorkflowStatusFailed))
	rt.ApplyMode = ApplyModeConfirm

	if err := rt.Run(Request{
		RequestID: "req-1", SessionID: "sess-1",
		Operation: "fix_execution", ExecutionID: "exec-1", Message: "修复",
	}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	return rt, sessions, wf, emit
}

// TestStagePendingWorkflow 验证确认模式下不落盘、草案入会话、事件类型正确。
func TestStagePendingWorkflow(t *testing.T) {
	_, sessions, wf, emit := stagedRuntime(t)

	// 工作流不应被保存（仍是原 command）
	if wf.saved.Nodes[0].Config["command"] != "deploy.sh" {
		t.Fatalf("确认模式不应直接落盘: %#v", wf.saved)
	}
	// 草案在会话上
	pending := sessions.data["sess-1"].PendingDraft
	if pending == nil || pending.ArtifactID != "wf-1" || pending.BaseHash == "" {
		t.Fatalf("PendingDraft 异常: %#v", pending)
	}
	if !strings.Contains(pending.ChangeSummary, "修改") {
		t.Fatalf("diff 摘要异常: %s", pending.ChangeSummary)
	}
	// 事件序列应含 workflow_pending + done，且无 workflow(update)
	var seenPending, seenUpdate bool
	for _, e := range emit.events {
		if e.Type == EventWorkflowPending {
			seenPending = true
		}
		if e.Type == EventWorkflow {
			seenUpdate = true
		}
	}
	if !seenPending || seenUpdate {
		t.Fatalf("事件异常 pending=%v update=%v: %#v", seenPending, seenUpdate, emit.events)
	}
}

// TestApplyPendingDraft 验证应用草案：落盘 + 清除草案 + 追加结果消息。
func TestApplyPendingDraft(t *testing.T) {
	rt, sessions, wf, _ := stagedRuntime(t)

	applied, err := rt.ApplyPendingDraft("sess-1")
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if findNodeCommand(applied.Nodes) != "/opt/app/deploy.sh" {
		t.Fatalf("应用结果异常: %#v", applied)
	}
	if findNodeCommand(wf.saved.Nodes) != "/opt/app/deploy.sh" {
		t.Fatalf("草案未落盘: %#v", wf.saved)
	}
	final := sessions.data["sess-1"]
	if final.PendingDraft != nil {
		t.Fatal("应用后草案应清除")
	}
	last := final.Messages[len(final.Messages)-1]
	if !strings.Contains(last.Content, "已应用") {
		t.Fatalf("缺少应用结果消息: %s", last.Content)
	}
}

// TestApplyPendingDraftBaselineDrift 验证基线漂移时拒绝应用且保留草案。
func TestApplyPendingDraftBaselineDrift(t *testing.T) {
	rt, sessions, wf, _ := stagedRuntime(t)

	// 模拟用户在草案生成后手工修改了工作流
	drifted := *wf.saved
	drifted.Name = "手工改过"
	wf.saved = &drifted

	if _, err := rt.ApplyPendingDraft("sess-1"); err == nil || !strings.Contains(err.Error(), "被修改过") {
		t.Fatalf("应拒绝基线漂移的应用，got %v", err)
	}
	if sessions.data["sess-1"].PendingDraft == nil {
		t.Fatal("漂移拒绝后草案应保留")
	}
}

// TestDiscardPendingDraft 验证放弃草案的幂等性。
func TestDiscardPendingDraft(t *testing.T) {
	rt, sessions, wf, _ := stagedRuntime(t)

	if err := rt.DiscardPendingDraft("sess-1"); err != nil {
		t.Fatalf("Discard error: %v", err)
	}
	if sessions.data["sess-1"].PendingDraft != nil {
		t.Fatal("放弃后草案应清除")
	}
	if wf.saved.Nodes[0].Config["command"] != "deploy.sh" {
		t.Fatal("放弃不应改动工作流")
	}
	// 幂等：再次放弃不报错
	if err := rt.DiscardPendingDraft("sess-1"); err != nil {
		t.Fatalf("重复放弃应幂等: %v", err)
	}
}
