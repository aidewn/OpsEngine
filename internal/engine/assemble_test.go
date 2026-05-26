// 集合调用执行的集成测试

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

// TestAssembleCall_ParamReturn 工作流调用集合，传参 + 收返回值
func TestAssembleCall_ParamReturn(t *testing.T) {
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)

	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)

	// 集合：参数 input，返回 result
	// 内部：start → print(param("input")) → end(return_result=param("input"))
	asm := core.AssembleDef{
		ID:   "asm-1",
		Name: "Echo",
		Params: []core.ParamDef{
			{Name: "input", VarType: core.PortTypeString},
		},
		Returns: []core.ParamDef{
			{Name: "result", VarType: core.PortTypeString},
		},
		Variables: []core.VariableDef{},
		Nodes: []core.NodeInstance{
			{InstanceID: "start", TypeID: "assemble_start", Config: map[string]any{}},
			{InstanceID: "param_in", TypeID: "assemble_param", Config: map[string]any{
				"param_name": "input", "var_type": "String",
			}},
			{InstanceID: "print", TypeID: "print", Config: map[string]any{
				"prefix": "[ASM]",
			}},
			{InstanceID: "end", TypeID: "assemble_end", Config: map[string]any{}},
		},
		Edges: []core.EdgeConfig{
			// exec 流：start → print → end
			{From: core.PortRef{Node: "start", Port: "exec_out"},
				To: core.PortRef{Node: "print", Port: "exec_in"}},
			{From: core.PortRef{Node: "print", Port: "exec_out"},
				To: core.PortRef{Node: "end", Port: "exec_in"}},
			// 数据流：param("input") → print.message
			{From: core.PortRef{Node: "param_in", Port: "value"},
				To: core.PortRef{Node: "print", Port: "message"}},
			// 数据流：param("input") → end.return_result（值原样回传）
			{From: core.PortRef{Node: "param_in", Port: "value"},
				To: core.PortRef{Node: "end", Port: "return_result"}},
		},
	}
	if err := as.Save(asm); err != nil {
		t.Fatal(err)
	}

	// 工作流：ready → var_get(msg) → call(asm-1) → print(result)
	wf := core.WorkflowDef{
		ID:   "wf-1",
		Name: "调用集合",
		Variables: []core.VariableDef{
			{Name: "msg", VarType: core.PortTypeString, Default: "Hello Assemble"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready", Config: map[string]any{}},
			{InstanceID: "get", TypeID: "var_get", Config: map[string]any{
				"var_name": "msg", "var_type": "String",
			}},
			{InstanceID: "call", TypeID: "assemble:asm-1", Config: map[string]any{}},
			{InstanceID: "print2", TypeID: "print", Config: map[string]any{
				"prefix": "[MAIN]",
			}},
		},
		Edges: []core.EdgeConfig{
			// exec 流：ready → call → print2
			{From: core.PortRef{Node: "ready", Port: "exec_out"},
				To: core.PortRef{Node: "call", Port: "exec_in"}},
			{From: core.PortRef{Node: "call", Port: "exec_out"},
				To: core.PortRef{Node: "print2", Port: "exec_in"}},
			// 数据流：get(msg) → call.param_input
			{From: core.PortRef{Node: "get", Port: "value"},
				To: core.PortRef{Node: "call", Port: "param_input"}},
			// 数据流：call.return_result → print2.message
			{From: core.PortRef{Node: "call", Port: "return_result"},
				To: core.PortRef{Node: "print2", Port: "message"}},
		},
	}
	if err := ws.Save(wf); err != nil {
		t.Fatal(err)
	}

	// 跑
	collector := &collectorEmitter{}
	e := engine.New(ws, as, nil, nil, collector)
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

	// 集合内的 print 在子 frame（caller = "call"）中
	callFrame := record.RootFrame.Children["call"]
	if callFrame == nil {
		t.Fatal("找不到集合 frame")
	}
	asmPrintLogs := callFrame.NodeLogs["print"]
	if len(asmPrintLogs) == 0 {
		t.Fatal("集合内 print 节点无日志")
	}
	if asmPrintLogs[0].Message != "[ASM] Hello Assemble" {
		t.Errorf("集合 print 期望 '[ASM] Hello Assemble'，实际: %q", asmPrintLogs[0].Message)
	}

	// 主流的 print2 应该输出 "[MAIN] Hello Assemble"
	mainPrintLogs := record.RootFrame.NodeLogs["print2"]
	if len(mainPrintLogs) == 0 {
		t.Fatal("主流 print2 节点无日志")
	}
	if mainPrintLogs[0].Message != "[MAIN] Hello Assemble" {
		t.Errorf("主流 print2 期望 '[MAIN] Hello Assemble'，实际: %q", mainPrintLogs[0].Message)
	}

	t.Logf("主流节点状态: %+v", record.RootFrame.NodeStates)
	t.Logf("集合节点状态: %+v", callFrame.NodeStates)
}

// TestAssembleCall_VariableIsolation 验证集合变量与主流变量互不影响
func TestAssembleCall_VariableIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)

	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)

	// 集合内有自己的变量 x="asm-val"，set x="asm-changed"
	asm := core.AssembleDef{
		ID:   "asm-iso",
		Name: "Iso",
		Variables: []core.VariableDef{
			{Name: "x", VarType: core.PortTypeString, Default: "asm-val"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "start", TypeID: "assemble_start", Config: map[string]any{}},
			{InstanceID: "src", TypeID: "var_get", Config: map[string]any{
				"var_name": "x", "var_type": "String",
			}},
			// 改一个临时变量装新值
			{InstanceID: "newval_src", TypeID: "var_get", Config: map[string]any{
				"var_name": "x", "var_type": "String",
			}},
			{InstanceID: "set", TypeID: "var_set", Config: map[string]any{
				"var_name": "x", "var_type": "String",
			}},
			{InstanceID: "end", TypeID: "assemble_end", Config: map[string]any{}},
		},
		Edges: []core.EdgeConfig{
			{From: core.PortRef{Node: "start", Port: "exec_out"},
				To: core.PortRef{Node: "set", Port: "exec_in"}},
			{From: core.PortRef{Node: "set", Port: "exec_out"},
				To: core.PortRef{Node: "end", Port: "exec_in"}},
			// 这里硬编码一个 input 通过 var_get 拼凑（实际用文本节点更好，但 MVP 没有）
			{From: core.PortRef{Node: "newval_src", Port: "value"},
				To: core.PortRef{Node: "set", Port: "value"}},
		},
	}
	if err := as.Save(asm); err != nil {
		t.Fatal(err)
	}
	_ = asm

	// 主流：定义同名变量 x="main-val"，调用集合后 print(x)
	wf := core.WorkflowDef{
		ID:   "wf-iso",
		Name: "变量隔离",
		Variables: []core.VariableDef{
			{Name: "x", VarType: core.PortTypeString, Default: "main-val"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready", Config: map[string]any{}},
			{InstanceID: "call", TypeID: "assemble:asm-iso", Config: map[string]any{}},
			{InstanceID: "get_x", TypeID: "var_get", Config: map[string]any{
				"var_name": "x", "var_type": "String",
			}},
			{InstanceID: "print", TypeID: "print", Config: map[string]any{}},
		},
		Edges: []core.EdgeConfig{
			{From: core.PortRef{Node: "ready", Port: "exec_out"},
				To: core.PortRef{Node: "call", Port: "exec_in"}},
			{From: core.PortRef{Node: "call", Port: "exec_out"},
				To: core.PortRef{Node: "print", Port: "exec_in"}},
			{From: core.PortRef{Node: "get_x", Port: "value"},
				To: core.PortRef{Node: "print", Port: "message"}},
		},
	}
	if err := ws.Save(wf); err != nil {
		t.Fatal(err)
	}

	collector := &collectorEmitter{}
	e := engine.New(ws, as, nil, nil, collector)
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
	// 主流的 x 应该仍是 main-val，没被集合内部的 set 影响
	if v := record.RootFrame.Variables["x"]; v != "main-val" {
		t.Errorf("主流变量 x 应为 \"main-val\"，实际 %v", v)
	}

	// print 日志应该是 "[INFO] main-val"
	logs := record.RootFrame.NodeLogs["print"]
	if len(logs) == 0 {
		t.Fatal("主流 print 无日志")
	}
	want := "[INFO] main-val"
	if logs[0].Message != want {
		t.Errorf("print 日志期望 %q，实际 %q", want, logs[0].Message)
	}
}

// TestAssembleCall_StartParamPort 从 assemble_start.param_* 直接取参（无需 assemble_param 节点）
func TestAssembleCall_StartParamPort(t *testing.T) {
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)

	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)

	asm := core.AssembleDef{
		ID:   "asm-start-param",
		Name: "StartParam",
		Params: []core.ParamDef{
			{Name: "input", VarType: core.PortTypeString},
		},
		Returns: []core.ParamDef{
			{Name: "result", VarType: core.PortTypeString},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "start", TypeID: "assemble_start", Config: map[string]any{}},
			{InstanceID: "print", TypeID: "print", Config: map[string]any{"prefix": "[ASM]"}},
			{InstanceID: "end", TypeID: "assemble_end", Config: map[string]any{}},
		},
		Edges: []core.EdgeConfig{
			{From: core.PortRef{Node: "start", Port: "exec_out"},
				To: core.PortRef{Node: "print", Port: "exec_in"}},
			{From: core.PortRef{Node: "print", Port: "exec_out"},
				To: core.PortRef{Node: "end", Port: "exec_in"}},
			{From: core.PortRef{Node: "start", Port: "param_input"},
				To: core.PortRef{Node: "print", Port: "message"}},
			{From: core.PortRef{Node: "start", Port: "param_input"},
				To: core.PortRef{Node: "end", Port: "return_result"}},
		},
	}
	if err := as.Save(asm); err != nil {
		t.Fatal(err)
	}

	wf := core.WorkflowDef{
		ID:   "wf-start-param",
		Name: "StartParamCall",
		Variables: []core.VariableDef{
			{Name: "msg", VarType: core.PortTypeString, Default: "via-start"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready", Config: map[string]any{}},
			{InstanceID: "get", TypeID: "var_get", Config: map[string]any{
				"var_name": "msg", "var_type": "String",
			}},
			{InstanceID: "call", TypeID: "assemble:asm-start-param", Config: map[string]any{}},
			{InstanceID: "print2", TypeID: "print", Config: map[string]any{"prefix": "[MAIN]"}},
		},
		Edges: []core.EdgeConfig{
			{From: core.PortRef{Node: "ready", Port: "exec_out"},
				To: core.PortRef{Node: "call", Port: "exec_in"}},
			{From: core.PortRef{Node: "call", Port: "exec_out"},
				To: core.PortRef{Node: "print2", Port: "exec_in"}},
			{From: core.PortRef{Node: "get", Port: "value"},
				To: core.PortRef{Node: "call", Port: "param_input"}},
			{From: core.PortRef{Node: "call", Port: "return_result"},
				To: core.PortRef{Node: "print2", Port: "message"}},
		},
	}
	if err := ws.Save(wf); err != nil {
		t.Fatal(err)
	}

	collector := &collectorEmitter{}
	e := engine.New(ws, as, nil, nil, collector)
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
	callFrame := record.RootFrame.Children["call"]
	if callFrame == nil {
		t.Fatal("找不到集合 frame")
	}
	if len(callFrame.NodeLogs["print"]) == 0 ||
		callFrame.NodeLogs["print"][0].Message != "[ASM] via-start" {
		t.Errorf("集合 print 日志不对: %v", callFrame.NodeLogs["print"])
	}
	if len(record.RootFrame.NodeLogs["print2"]) == 0 ||
		record.RootFrame.NodeLogs["print2"][0].Message != "[MAIN] via-start" {
		t.Errorf("主流 print2 日志不对: %v", record.RootFrame.NodeLogs["print2"])
	}
}
