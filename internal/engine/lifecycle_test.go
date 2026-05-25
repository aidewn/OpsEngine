// Phase 6: system_update 周期触发 + system_over 终止钩子 的集成测试

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

// TestSystemUpdate_Interval interval 触发：跑 1.5s 后停止，期望 print 至少 2 次
func TestSystemUpdate_Interval(t *testing.T) {
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)
	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)

	// 工作流：update(interval 500ms) → print "tick"
	wf := core.WorkflowDef{
		ID: "wf-update",
		Variables: []core.VariableDef{
			{Name: "m", VarType: core.PortTypeString, Default: "tick"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "upd", TypeID: "system_update", Config: map[string]any{
				"delta_type": "interval", "delta_seconds": int64(1), // 实际 1s，但我们用更小的 ticker 验证
			}},
			{InstanceID: "p", TypeID: "print"},
			{InstanceID: "g", TypeID: "var_get", Config: map[string]any{
				"var_name": "m", "var_type": "String",
			}},
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

	// 跑 2.5s 然后停止，至少应该触发 2 次
	time.Sleep(2500 * time.Millisecond)
	if err := e.Stop(execID); err != nil {
		t.Fatal(err)
	}

	// 等终态
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
		t.Fatalf("期望 Terminated，实际 %s", got)
	}

	logs := rt.Record().RootFrame.NodeLogs["p"]
	if len(logs) < 2 {
		t.Errorf("期望至少 2 条 print 日志，实际 %d 条: %+v", len(logs), logs)
	}
	t.Logf("收到 %d 条 print 日志", len(logs))
}

// TestSystemOver_Triggered 用户 Stop 后 system_over 流被执行
func TestSystemOver_Triggered(t *testing.T) {
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)
	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)

	// 工作流：ready → print "main"
	//        update(interval 10s) →（保持 runtime 活着）
	//        over → print "over"
	wf := core.WorkflowDef{
		ID: "wf-over",
		Variables: []core.VariableDef{
			{Name: "mm", VarType: core.PortTypeString, Default: "main"},
			{Name: "mo", VarType: core.PortTypeString, Default: "over"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready"},
			{InstanceID: "upd", TypeID: "system_update", Config: map[string]any{
				"enabled": "on", "delta_type": "interval", "delta_seconds": int64(10),
			}},
			{InstanceID: "over", TypeID: "system_over"},
			{InstanceID: "pmain", TypeID: "print"},
			{InstanceID: "pover", TypeID: "print"},
			{InstanceID: "gm", TypeID: "var_get", Config: map[string]any{"var_name": "mm", "var_type": "String"}},
			{InstanceID: "go", TypeID: "var_get", Config: map[string]any{"var_name": "mo", "var_type": "String"}},
		},
		Edges: []core.EdgeConfig{
			{From: core.PortRef{Node: "ready", Port: "exec_out"}, To: core.PortRef{Node: "pmain", Port: "exec_in"}},
			{From: core.PortRef{Node: "over", Port: "exec_out"}, To: core.PortRef{Node: "pover", Port: "exec_in"}},
			{From: core.PortRef{Node: "gm", Port: "value"}, To: core.PortRef{Node: "pmain", Port: "message"}},
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

	// 等 200ms 让主流跑完，然后 Stop
	time.Sleep(200 * time.Millisecond)
	if err := e.Stop(execID); err != nil {
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
		t.Fatalf("期望 Terminated，实际 %s", got)
	}

	rec := rt.Record()
	mainLogs := rec.RootFrame.NodeLogs["pmain"]
	overLogs := rec.RootFrame.NodeLogs["pover"]
	if len(mainLogs) == 0 || mainLogs[0].Message != "[INFO] main" {
		t.Errorf("主流 print 异常: %+v", mainLogs)
	}
	if len(overLogs) == 0 || overLogs[0].Message != "[INFO] over" {
		t.Errorf("over 流 print 异常: %+v", overLogs)
	}
}

// TestSystemOver_NaturalEnd 主流自然结束（无 update）时也跑 system_over
func TestSystemOver_NaturalEnd(t *testing.T) {
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)
	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)

	wf := core.WorkflowDef{
		ID: "wf-over-natural",
		Variables: []core.VariableDef{
			{Name: "mm", VarType: core.PortTypeString, Default: "main"},
			{Name: "mo", VarType: core.PortTypeString, Default: "cleanup"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready"},
			{InstanceID: "over", TypeID: "system_over"},
			{InstanceID: "pmain", TypeID: "print"},
			{InstanceID: "pover", TypeID: "print"},
			{InstanceID: "gm", TypeID: "var_get", Config: map[string]any{"var_name": "mm", "var_type": "String"}},
			{InstanceID: "go", TypeID: "var_get", Config: map[string]any{"var_name": "mo", "var_type": "String"}},
		},
		Edges: []core.EdgeConfig{
			{From: core.PortRef{Node: "ready", Port: "exec_out"}, To: core.PortRef{Node: "pmain", Port: "exec_in"}},
			{From: core.PortRef{Node: "over", Port: "exec_out"}, To: core.PortRef{Node: "pover", Port: "exec_in"}},
			{From: core.PortRef{Node: "gm", Port: "value"}, To: core.PortRef{Node: "pmain", Port: "message"}},
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
		time.Sleep(10 * time.Millisecond)
	}

	rt, _ := e.Get(execID)
	// 主流自然完成 + over 流也跑了 → Success
	if got := rt.Status(); got != core.WorkflowStatusSuccess {
		t.Fatalf("期望 Success，实际 %s", got)
	}

	rec := rt.Record()
	overLogs := rec.RootFrame.NodeLogs["pover"]
	if len(overLogs) == 0 || overLogs[0].Message != "[INFO] cleanup" {
		t.Errorf("over 流应执行 print cleanup，实际: %+v", overLogs)
	}
}
