// Phase 5：并发 + 线程节点的集成测试

package engine_test

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	_ "OpsEngine/internal/nodes"
	"OpsEngine/internal/store"
)

// TestParallel_AllBranchesRun parallel 节点触发后所有分支都执行，最终走 exec_out_done
func TestParallel_AllBranchesRun(t *testing.T) {
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)
	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)

	// ready → parallel → [print A, print B, print C] → done → print Done
	wf := core.WorkflowDef{
		ID: "wf-parallel",
		Variables: []core.VariableDef{
			{Name: "msg_a", VarType: core.PortTypeString, Default: "A"},
			{Name: "msg_b", VarType: core.PortTypeString, Default: "B"},
			{Name: "msg_c", VarType: core.PortTypeString, Default: "C"},
			{Name: "msg_done", VarType: core.PortTypeString, Default: "Done"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready"},
			{InstanceID: "par", TypeID: "parallel"},
			{InstanceID: "pa", TypeID: "print"},
			{InstanceID: "pb", TypeID: "print"},
			{InstanceID: "pc", TypeID: "print"},
			{InstanceID: "pd", TypeID: "print"},
			{InstanceID: "ga", TypeID: "var_get", Config: map[string]any{"var_name": "msg_a", "var_type": "String"}},
			{InstanceID: "gb", TypeID: "var_get", Config: map[string]any{"var_name": "msg_b", "var_type": "String"}},
			{InstanceID: "gc", TypeID: "var_get", Config: map[string]any{"var_name": "msg_c", "var_type": "String"}},
			{InstanceID: "gd", TypeID: "var_get", Config: map[string]any{"var_name": "msg_done", "var_type": "String"}},
		},
		Edges: []core.EdgeConfig{
			{From: core.PortRef{Node: "ready", Port: "exec_out"}, To: core.PortRef{Node: "par", Port: "exec_in"}},
			{From: core.PortRef{Node: "par", Port: "exec_out_1"}, To: core.PortRef{Node: "pa", Port: "exec_in"}},
			{From: core.PortRef{Node: "par", Port: "exec_out_2"}, To: core.PortRef{Node: "pb", Port: "exec_in"}},
			{From: core.PortRef{Node: "par", Port: "exec_out_3"}, To: core.PortRef{Node: "pc", Port: "exec_in"}},
			{From: core.PortRef{Node: "par", Port: "exec_out_done"}, To: core.PortRef{Node: "pd", Port: "exec_in"}},
			{From: core.PortRef{Node: "ga", Port: "value"}, To: core.PortRef{Node: "pa", Port: "message"}},
			{From: core.PortRef{Node: "gb", Port: "value"}, To: core.PortRef{Node: "pb", Port: "message"}},
			{From: core.PortRef{Node: "gc", Port: "value"}, To: core.PortRef{Node: "pc", Port: "message"}},
			{From: core.PortRef{Node: "gd", Port: "value"}, To: core.PortRef{Node: "pd", Port: "message"}},
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
	waitDone(t, e, execID, 3*time.Second)

	rt, _ := e.Get(execID)
	if got := rt.Status(); got != core.WorkflowStatusSuccess {
		t.Fatalf("期望 Success，实际 %s（错误: %s）", got, rt.Record().Error)
	}

	rec := rt.Record()
	// 三个分支的 print 节点应该都跑了
	checks := []struct{ id, want string }{
		{"pa", "[INFO] A"}, {"pb", "[INFO] B"}, {"pc", "[INFO] C"},
		{"pd", "[INFO] Done"},
	}
	for _, c := range checks {
		logs := rec.RootFrame.NodeLogs[c.id]
		if len(logs) == 0 {
			t.Errorf("节点 %s 未输出日志", c.id)
			continue
		}
		if logs[0].Message != c.want {
			t.Errorf("节点 %s 日志期望 %q，实际 %q", c.id, c.want, logs[0].Message)
		}
	}
	t.Logf("节点状态: %+v", rec.RootFrame.NodeStates)
}

// TestParallel_ConcurrentWrites 并发分支写同一变量不应 panic
func TestParallel_ConcurrentWrites(t *testing.T) {
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)
	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)

	// 3 个分支同时 set 同一变量
	wf := core.WorkflowDef{
		ID: "wf-race",
		Variables: []core.VariableDef{
			{Name: "shared", VarType: core.PortTypeString, Default: "init"},
			{Name: "v1", VarType: core.PortTypeString, Default: "v1"},
			{Name: "v2", VarType: core.PortTypeString, Default: "v2"},
			{Name: "v3", VarType: core.PortTypeString, Default: "v3"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready"},
			{InstanceID: "par", TypeID: "parallel"},
			{InstanceID: "s1", TypeID: "var_set", Config: map[string]any{"var_name": "shared", "var_type": "String"}},
			{InstanceID: "s2", TypeID: "var_set", Config: map[string]any{"var_name": "shared", "var_type": "String"}},
			{InstanceID: "s3", TypeID: "var_set", Config: map[string]any{"var_name": "shared", "var_type": "String"}},
			{InstanceID: "g1", TypeID: "var_get", Config: map[string]any{"var_name": "v1", "var_type": "String"}},
			{InstanceID: "g2", TypeID: "var_get", Config: map[string]any{"var_name": "v2", "var_type": "String"}},
			{InstanceID: "g3", TypeID: "var_get", Config: map[string]any{"var_name": "v3", "var_type": "String"}},
		},
		Edges: []core.EdgeConfig{
			{From: core.PortRef{Node: "ready", Port: "exec_out"}, To: core.PortRef{Node: "par", Port: "exec_in"}},
			{From: core.PortRef{Node: "par", Port: "exec_out_1"}, To: core.PortRef{Node: "s1", Port: "exec_in"}},
			{From: core.PortRef{Node: "par", Port: "exec_out_2"}, To: core.PortRef{Node: "s2", Port: "exec_in"}},
			{From: core.PortRef{Node: "par", Port: "exec_out_3"}, To: core.PortRef{Node: "s3", Port: "exec_in"}},
			{From: core.PortRef{Node: "g1", Port: "value"}, To: core.PortRef{Node: "s1", Port: "value"}},
			{From: core.PortRef{Node: "g2", Port: "value"}, To: core.PortRef{Node: "s2", Port: "value"}},
			{From: core.PortRef{Node: "g3", Port: "value"}, To: core.PortRef{Node: "s3", Port: "value"}},
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
	waitDone(t, e, execID, 3*time.Second)

	rt, _ := e.Get(execID)
	if got := rt.Status(); got != core.WorkflowStatusSuccess {
		t.Fatalf("期望 Success，实际 %s", got)
	}
	// shared 终态是 v1/v2/v3 之一（取决于谁最后写）
	v := rt.Record().RootFrame.Variables["shared"]
	valid := map[string]bool{"v1": true, "v2": true, "v3": true}
	if !valid[v.(string)] {
		t.Errorf("shared 终态应为 v1/v2/v3 之一，实际 %v", v)
	}
}

// TestThread_MainNotBlocked 线程节点不阻塞主流
func TestThread_MainNotBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)
	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)

	// ready → thread → continue: print "main"
	//                 → thread:   print "bg"
	// 主流应该立即跑 print main 然后结束，bg 线程跟上
	wf := core.WorkflowDef{
		ID: "wf-thread",
		Variables: []core.VariableDef{
			{Name: "m", VarType: core.PortTypeString, Default: "main"},
			{Name: "b", VarType: core.PortTypeString, Default: "bg"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready"},
			{InstanceID: "th", TypeID: "thread"},
			{InstanceID: "pmain", TypeID: "print"},
			{InstanceID: "pbg", TypeID: "print"},
			{InstanceID: "gm", TypeID: "var_get", Config: map[string]any{"var_name": "m", "var_type": "String"}},
			{InstanceID: "gb", TypeID: "var_get", Config: map[string]any{"var_name": "b", "var_type": "String"}},
		},
		Edges: []core.EdgeConfig{
			{From: core.PortRef{Node: "ready", Port: "exec_out"}, To: core.PortRef{Node: "th", Port: "exec_in"}},
			{From: core.PortRef{Node: "th", Port: "exec_out_continue"}, To: core.PortRef{Node: "pmain", Port: "exec_in"}},
			{From: core.PortRef{Node: "th", Port: "exec_out_thread"}, To: core.PortRef{Node: "pbg", Port: "exec_in"}},
			{From: core.PortRef{Node: "gm", Port: "value"}, To: core.PortRef{Node: "pmain", Port: "message"}},
			{From: core.PortRef{Node: "gb", Port: "value"}, To: core.PortRef{Node: "pbg", Port: "message"}},
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
	waitDone(t, e, execID, 3*time.Second)

	rt, _ := e.Get(execID)
	if got := rt.Status(); got != core.WorkflowStatusSuccess {
		t.Fatalf("期望 Success，实际 %s", got)
	}
	rec := rt.Record()
	mainLog := rec.RootFrame.NodeLogs["pmain"]
	bgLog := rec.RootFrame.NodeLogs["pbg"]
	if len(mainLog) == 0 || mainLog[0].Message != "[INFO] main" {
		t.Errorf("main 日志异常: %+v", mainLog)
	}
	if len(bgLog) == 0 || bgLog[0].Message != "[INFO] bg" {
		t.Errorf("bg 日志异常: %+v", bgLog)
	}
}

// ── 辅助 ─────────────────────────────────────────────────

func waitDone(t *testing.T, e *engine.Engine, execID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		rt, ok := e.Get(execID)
		if !ok {
			t.Fatal("Runtime 不存在")
		}
		if rt.Status() != core.WorkflowStatusRunning {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待执行结束超时")
}

// 防止 collectorEmitter 在并发下未导出导致 lint 错误
var _ = sort.Slice
var _ = sync.WaitGroup{}
