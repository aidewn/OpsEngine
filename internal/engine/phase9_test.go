// Phase 9: system_update enabled 三态 + break 节点 + NodeStateTerminated

package engine_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	_ "OpsEngine/internal/nodes"
	"OpsEngine/internal/store"
)

// TestSystemUpdate_AutoNoConnection auto + 无 exec_out 连接 → 不启动 scheduler，主流跑完即结束
func TestSystemUpdate_AutoNoConnection(t *testing.T) {
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)
	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)

	// 含 system_update 但没连任何下游 → runtime 应该自然结束
	wf := core.WorkflowDef{
		ID: "wf-auto",
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready"},
			{InstanceID: "upd", TypeID: "system_update", Config: map[string]any{
				"enabled": "auto", "delta_type": "interval", "delta_seconds": int64(1),
			}},
		},
		Edges: []core.EdgeConfig{},
	}
	if err := ws.Save(wf); err != nil {
		t.Fatal(err)
	}

	e := engine.New(ws, as, nil, &collectorEmitter{})
	execID, err := e.Run(wf.ID)
	if err != nil {
		t.Fatal(err)
	}

	// 等终态（最多 1s 内应该结束）
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		rt, _ := e.Get(execID)
		if rt.Status() != core.WorkflowStatusRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	rt, _ := e.Get(execID)
	if got := rt.Status(); got != core.WorkflowStatusSuccess {
		t.Fatalf("auto+无连接应当 Success，实际 %s", got)
	}
}

// TestSystemUpdate_Off 即使有连接，off 也不启动
func TestSystemUpdate_Off(t *testing.T) {
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)
	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)

	wf := core.WorkflowDef{
		ID: "wf-off",
		Variables: []core.VariableDef{
			{Name: "m", VarType: core.PortTypeString, Default: "tick"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "upd", TypeID: "system_update", Config: map[string]any{
				"enabled": "off", "delta_type": "interval", "delta_seconds": int64(1),
			}},
			{InstanceID: "p", TypeID: "print"},
			{InstanceID: "g", TypeID: "var_get", Config: map[string]any{"var_name": "m", "var_type": "String"}},
		},
		Edges: []core.EdgeConfig{
			{From: core.PortRef{Node: "upd", Port: "exec_out"}, To: core.PortRef{Node: "p", Port: "exec_in"}},
			{From: core.PortRef{Node: "g", Port: "value"}, To: core.PortRef{Node: "p", Port: "message"}},
		},
	}
	if err := ws.Save(wf); err != nil {
		t.Fatal(err)
	}

	e := engine.New(ws, as, nil, &collectorEmitter{})
	execID, err := e.Run(wf.ID)
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		rt, _ := e.Get(execID)
		if rt.Status() != core.WorkflowStatusRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	rt, _ := e.Get(execID)
	if got := rt.Status(); got != core.WorkflowStatusSuccess {
		t.Fatalf("off 应当立即 Success，实际 %s", got)
	}
	// 不应该跑 print（ticker 没启动）
	logs := rt.Record().RootFrame.NodeLogs["p"]
	if len(logs) > 0 {
		t.Errorf("off 状态下 update 不应触发，实际触发了 %d 次", len(logs))
	}
}

// TestBreak_TerminatesWorkflow break 节点触发整个工作流终止
func TestBreak_TerminatesWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)
	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)

	// ready → break → print（不应执行）
	// over → print("over") 应执行（system_over 流必跑）
	wf := core.WorkflowDef{
		ID: "wf-break",
		Variables: []core.VariableDef{
			{Name: "n", VarType: core.PortTypeString, Default: "never"},
			{Name: "o", VarType: core.PortTypeString, Default: "over"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready"},
			{InstanceID: "brk", TypeID: "break"},
			{InstanceID: "pnever", TypeID: "print"},
			{InstanceID: "over", TypeID: "system_over"},
			{InstanceID: "pover", TypeID: "print"},
			{InstanceID: "gn", TypeID: "var_get", Config: map[string]any{"var_name": "n", "var_type": "String"}},
			{InstanceID: "go", TypeID: "var_get", Config: map[string]any{"var_name": "o", "var_type": "String"}},
		},
		Edges: []core.EdgeConfig{
			{From: core.PortRef{Node: "ready", Port: "exec_out"}, To: core.PortRef{Node: "brk", Port: "exec_in"}},
			// brk 没有 exec_out，pnever 不会被触发
			{From: core.PortRef{Node: "over", Port: "exec_out"}, To: core.PortRef{Node: "pover", Port: "exec_in"}},
			{From: core.PortRef{Node: "gn", Port: "value"}, To: core.PortRef{Node: "pnever", Port: "message"}},
			{From: core.PortRef{Node: "go", Port: "value"}, To: core.PortRef{Node: "pover", Port: "message"}},
		},
	}
	if err := ws.Save(wf); err != nil {
		t.Fatal(err)
	}

	e := engine.New(ws, as, nil, &collectorEmitter{})
	execID, err := e.Run(wf.ID)
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rt, _ := e.Get(execID)
		if rt.Status() != core.WorkflowStatusRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	rt, _ := e.Get(execID)
	if got := rt.Status(); got != core.WorkflowStatusTerminated {
		t.Fatalf("break 后应当 Terminated，实际 %s", got)
	}
	rec := rt.Record()
	// pnever 不应被执行
	if logs := rec.RootFrame.NodeLogs["pnever"]; len(logs) > 0 {
		t.Errorf("break 后的节点不应执行，实际日志: %+v", logs)
	}
	// pover 应被执行
	if logs := rec.RootFrame.NodeLogs["pover"]; len(logs) == 0 || logs[0].Message != "[INFO] over" {
		t.Errorf("over 流应该执行，实际: %+v", logs)
	}
	// brk 自身状态应为 Success
	if s := rec.RootFrame.NodeStates["brk"]; s != core.NodeStateSuccess {
		t.Errorf("break 节点自身应为 Success，实际 %s", s)
	}
}
