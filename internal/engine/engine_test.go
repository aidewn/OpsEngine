// 引擎端到端集成测试：跑通 Hello World

package engine_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	_ "OpsEngine/internal/nodes" // 触发节点注册
	"OpsEngine/internal/store"
)

// collectorEmitter 把事件收集到列表里供测试断言
type collectorEmitter struct {
	mu     sync.Mutex
	events []event
}

type event struct {
	Name string
	Data any
}

func (c *collectorEmitter) Emit(name string, data any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event{Name: name, Data: data})
}

func (c *collectorEmitter) Events() []event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]event, len(c.events))
	copy(out, c.events)
	return out
}

// TestEngine_HelloWorld 完整跑通：system_ready → var_get(msg) → print
// 工作流变量 msg = "Hello World"
func TestEngine_HelloWorld(t *testing.T) {
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)

	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)

	// 构造工作流
	wf := core.WorkflowDef{
		ID:   "wf-hello",
		Name: "Hello World 测试",
		Variables: []core.VariableDef{
			{Name: "msg", VarType: core.PortTypeString, Default: "Hello World"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready", Config: map[string]any{}},
			{InstanceID: "get", TypeID: "var_get", Config: map[string]any{
				"var_name": "msg",
				"var_type": "String",
			}},
			{InstanceID: "print", TypeID: "print", Config: map[string]any{
				"prefix": "[TEST]",
			}},
		},
		Edges: []core.EdgeConfig{
			{From: core.PortRef{Node: "ready", Port: "exec_out"},
				To: core.PortRef{Node: "print", Port: "exec_in"}},
			{From: core.PortRef{Node: "get", Port: "value"},
				To: core.PortRef{Node: "print", Port: "message"}},
		},
	}
	if err := ws.Save(wf); err != nil {
		t.Fatal(err)
	}

	// 跑
	collector := &collectorEmitter{}
	e := engine.New(ws, as, nil, collector)
	execID, err := e.Run(wf.ID)
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}

	// 等结束（最多 2s）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rt, ok := e.Get(execID)
		if !ok {
			t.Fatal("Runtime 不存在")
		}
		if rt.Status() != core.WorkflowStatusRunning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	rt, _ := e.Get(execID)
	if got := rt.Status(); got != core.WorkflowStatusSuccess {
		t.Fatalf("期望 Success，实际 %s（错误: %s）", got, rt.Record().Error)
	}

	// 校验事件流
	events := collector.Events()
	t.Logf("收到 %d 条事件", len(events))
	for _, ev := range events {
		t.Logf("  %s %v", ev.Name, ev.Data)
	}

	// 应该有的事件类型
	want := map[string]bool{
		engine.EventStarted:  false,
		engine.EventStatus:   false,
		engine.EventNode:     false,
		engine.EventLog:      false,
		engine.EventFinished: false,
	}
	for _, ev := range events {
		if _, ok := want[ev.Name]; ok {
			want[ev.Name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("缺少事件: %s", name)
		}
	}

	// 校验日志中包含 Hello World
	record := rt.Record()
	logs := record.NodeLogs["print"]
	if len(logs) == 0 {
		t.Fatal("print 节点没有日志")
	}
	if logs[0].Message != "[TEST] Hello World" {
		t.Errorf("期望日志 '[TEST] Hello World'，实际: %q", logs[0].Message)
	}

	// 校验节点状态
	if record.NodeStates["ready"] != core.NodeStateSuccess {
		t.Errorf("ready 节点状态应为 Success，实际 %s", record.NodeStates["ready"])
	}
	if record.NodeStates["print"] != core.NodeStateSuccess {
		t.Errorf("print 节点状态应为 Success，实际 %s", record.NodeStates["print"])
	}
	// var_get 是 pure 节点，按需求值，没有 Executing → Success 状态变更
	// 这是正确行为
}

// TestEngine_VarSetGet 验证 var_set 写入 → var_get 读出
func TestEngine_VarSetGet(t *testing.T) {
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)

	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)

	// system_ready → var_set(counter, "42") → print(var_get(counter))
	// var_set.value 输入未连线 → 取零值 nil
	// 这里测试 SetVariable 接收 nil 也不崩
	// 改用：定义初始 counter=10，var_set 改成 99，最后 print 读到 99
	wf := core.WorkflowDef{
		ID:   "wf-varset",
		Name: "var_set 测试",
		Variables: []core.VariableDef{
			{Name: "counter", VarType: core.PortTypeString, Default: "10"},
			{Name: "newval", VarType: core.PortTypeString, Default: "99"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready", Config: map[string]any{}},
			{InstanceID: "src", TypeID: "var_get", Config: map[string]any{
				"var_name": "newval", "var_type": "String",
			}},
			{InstanceID: "set", TypeID: "var_set", Config: map[string]any{
				"var_name": "counter", "var_type": "String",
			}},
			{InstanceID: "get", TypeID: "var_get", Config: map[string]any{
				"var_name": "counter", "var_type": "String",
			}},
			{InstanceID: "print", TypeID: "print", Config: map[string]any{}},
		},
		Edges: []core.EdgeConfig{
			// exec 流
			{From: core.PortRef{Node: "ready", Port: "exec_out"},
				To: core.PortRef{Node: "set", Port: "exec_in"}},
			{From: core.PortRef{Node: "set", Port: "exec_out"},
				To: core.PortRef{Node: "print", Port: "exec_in"}},
			// 数据流
			{From: core.PortRef{Node: "src", Port: "value"},
				To: core.PortRef{Node: "set", Port: "value"}},
			{From: core.PortRef{Node: "get", Port: "value"},
				To: core.PortRef{Node: "print", Port: "message"}},
		},
	}
	if err := ws.Save(wf); err != nil {
		t.Fatal(err)
	}

	collector := &collectorEmitter{}
	e := engine.New(ws, as, nil, collector)
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
	if got := rt.Status(); got != core.WorkflowStatusSuccess {
		t.Fatalf("期望 Success，实际 %s（错误: %s）", got, rt.Record().Error)
	}

	record := rt.Record()
	// counter 应该被改成 "99"
	if v := record.Variables["counter"]; v != "99" {
		t.Errorf("counter 应为 \"99\"，实际 %v (%T)", v, v)
	}

	// print 日志最后应是 "[INFO] 99"
	logs := record.NodeLogs["print"]
	if len(logs) == 0 {
		t.Fatal("print 节点没有日志")
	}
	got := logs[len(logs)-1].Message
	want := "[INFO] 99"
	if got != want {
		t.Errorf("print 日志期望 %q，实际 %q", want, got)
	}
}
