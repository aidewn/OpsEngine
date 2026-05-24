// Phase 7: 执行记录持久化测试

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

// TestPersistence_SaveAndLoad 执行终态自动写盘，重新创建 Engine 后仍可加载
func TestPersistence_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	exDir := filepath.Join(tmpDir, "executions")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)
	_ = os.MkdirAll(exDir, 0755)
	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)
	es := store.NewExecutionStore(exDir)

	wf := core.WorkflowDef{
		ID:   "wf-persist",
		Name: "持久化测试",
		Variables: []core.VariableDef{
			{Name: "m", VarType: core.PortTypeString, Default: "Hello"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready"},
			{InstanceID: "p", TypeID: "print"},
			{InstanceID: "g", TypeID: "var_get", Config: map[string]any{
				"var_name": "m", "var_type": "String",
			}},
		},
		Edges: []core.EdgeConfig{
			{From: core.PortRef{Node: "ready", Port: "exec_out"}, To: core.PortRef{Node: "p", Port: "exec_in"}},
			{From: core.PortRef{Node: "g", Port: "value"}, To: core.PortRef{Node: "p", Port: "message"}},
		},
	}
	if err := ws.Save(wf); err != nil {
		t.Fatal(err)
	}

	// 第一个 Engine 跑一次
	e1 := engine.New(ws, as, es, &collectorEmitter{})
	execID, err := e1.Run(wf.ID)
	if err != nil {
		t.Fatal(err)
	}

	// 等终态
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rt, _ := e1.Get(execID)
		if rt.Status() != core.WorkflowStatusRunning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 文件应该已经写入
	expectedPath := filepath.Join(exDir, execID+".json")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("终态执行记录未写入磁盘: %v", err)
	}

	// 第二个 Engine（模拟重启），不调用 Run，直接 ListSummaries
	e2 := engine.New(ws, as, es, &collectorEmitter{})
	summaries := e2.ListSummaries()
	found := false
	for _, s := range summaries {
		if s.ID == execID {
			found = true
			if s.Status != core.WorkflowStatusSuccess {
				t.Errorf("持久化记录状态应为 Success，实际 %s", s.Status)
			}
			break
		}
	}
	if !found {
		t.Errorf("重新加载后未找到 execution %s", execID)
	}

	// GetRecord 也应工作
	rec, ok := e2.GetRecord(execID)
	if !ok {
		t.Fatal("GetRecord 找不到持久化记录")
	}
	logs := rec.NodeLogs["p"]
	if len(logs) == 0 || logs[0].Message != "[INFO] Hello" {
		t.Errorf("持久化的日志不正确: %+v", logs)
	}
}

// TestPersistence_DeleteRemovesFile Remove 同时删除磁盘文件
func TestPersistence_DeleteRemovesFile(t *testing.T) {
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	exDir := filepath.Join(tmpDir, "executions")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)
	_ = os.MkdirAll(exDir, 0755)
	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)
	es := store.NewExecutionStore(exDir)

	wf := core.WorkflowDef{
		ID: "wf-del",
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready"},
		},
	}
	if err := ws.Save(wf); err != nil {
		t.Fatal(err)
	}

	e := engine.New(ws, as, es, &collectorEmitter{})
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

	path := filepath.Join(exDir, execID+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("文件应存在: %v", err)
	}

	if err := e.Remove(execID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("文件应被删除，但仍存在: %v", err)
	}
}
