// Phase 8 Step 4：while_loop 节点条件循环测试
//
// 验证：
//   - CountUp：i 从 0 累加到 threshold (5)，condition=compare_lt(i, threshold) 在 i==5 时变 false 退出
//   - FirstFalse：i 一开始就 >= threshold，condition 首次即 false，body 不执行，但走 done

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

// buildWhileWorkflow 构造一个标准的 while-累加工作流
//
//	ready → while → body → set_i(value = add(get_i, get_one))
//	              → done → marker(print)
//	condition: lt(get_i, get_threshold)
//
// 通过 iStart 控制初始 i 值，threshold 固定为 5
func buildWhileWorkflow(id string, iStart int64) core.WorkflowDef {
	return core.WorkflowDef{
		ID: id,
		Variables: []core.VariableDef{
			{Name: "i", VarType: core.PortTypeInt, Default: iStart},
			{Name: "threshold", VarType: core.PortTypeInt, Default: int64(5)},
			{Name: "one", VarType: core.PortTypeInt, Default: int64(1)},
			{Name: "msg", VarType: core.PortTypeString, Default: "done"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready"},
			{InstanceID: "wl", TypeID: "while_loop"},
			{InstanceID: "set_i", TypeID: "var_set",
				Config: map[string]any{"var_name": "i", "var_type": "Int"}},
			{InstanceID: "add", TypeID: "math_add",
				Config: map[string]any{"var_type": "Int"}},
			{InstanceID: "lt", TypeID: "compare_lt",
				Config: map[string]any{"var_type": "Int"}},
			{InstanceID: "get_i", TypeID: "var_get",
				Config: map[string]any{"var_name": "i", "var_type": "Int"}},
			{InstanceID: "get_thresh", TypeID: "var_get",
				Config: map[string]any{"var_name": "threshold", "var_type": "Int"}},
			{InstanceID: "get_one", TypeID: "var_get",
				Config: map[string]any{"var_name": "one", "var_type": "Int"}},
			{InstanceID: "get_msg", TypeID: "var_get",
				Config: map[string]any{"var_name": "msg", "var_type": "String"}},
			{InstanceID: "marker", TypeID: "print"},
		},
		Edges: []core.EdgeConfig{
			// 主流
			{From: core.PortRef{Node: "ready", Port: "exec_out"}, To: core.PortRef{Node: "wl", Port: "exec_in"}},
			{From: core.PortRef{Node: "wl", Port: "exec_out_body"}, To: core.PortRef{Node: "set_i", Port: "exec_in"}},
			{From: core.PortRef{Node: "wl", Port: "exec_out_done"}, To: core.PortRef{Node: "marker", Port: "exec_in"}},
			// condition: lt(i, threshold)
			{From: core.PortRef{Node: "get_i", Port: "value"}, To: core.PortRef{Node: "lt", Port: "a"}},
			{From: core.PortRef{Node: "get_thresh", Port: "value"}, To: core.PortRef{Node: "lt", Port: "b"}},
			{From: core.PortRef{Node: "lt", Port: "result"}, To: core.PortRef{Node: "wl", Port: "condition"}},
			// body: i = i + 1
			{From: core.PortRef{Node: "get_i", Port: "value"}, To: core.PortRef{Node: "add", Port: "a"}},
			{From: core.PortRef{Node: "get_one", Port: "value"}, To: core.PortRef{Node: "add", Port: "b"}},
			{From: core.PortRef{Node: "add", Port: "result"}, To: core.PortRef{Node: "set_i", Port: "value"}},
			// done 分支日志
			{From: core.PortRef{Node: "get_msg", Port: "value"}, To: core.PortRef{Node: "marker", Port: "message"}},
		},
	}
}

// runWhileWorkflow 落盘 + 运行 + 等结束 + 返回执行记录
func runWhileWorkflow(t *testing.T, wf core.WorkflowDef) core.ExecutionRecord {
	t.Helper()
	tmpDir := t.TempDir()
	wfDir := filepath.Join(tmpDir, "workflows")
	asDir := filepath.Join(tmpDir, "assembles")
	_ = os.MkdirAll(wfDir, 0755)
	_ = os.MkdirAll(asDir, 0755)
	ws := store.NewWorkflowStore(wfDir)
	as := store.NewAssembleStore(asDir)

	if err := ws.Save(wf); err != nil {
		t.Fatal(err)
	}

	e := engine.New(ws, as, nil, nil, &collectorEmitter{})
	execID, err := e.Run(wf.ID)
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, e, execID, 3*time.Second)
	rt, _ := e.Get(execID)
	if got := rt.Status(); got != core.WorkflowStatusSuccess {
		t.Fatalf("期望 Success，实际 %s（错误: %s）", got, rt.Record().Error)
	}
	return rt.Record()
}

// TestWhileLoop_CountUp i 从 0 累加，condition=lt(i, 5) 在 i==5 时变 false 退出
func TestWhileLoop_CountUp(t *testing.T) {
	rec := runWhileWorkflow(t, buildWhileWorkflow("wf-while-countup", 0))

	got := rec.RootFrame.Variables["i"]
	want := int64(5)
	if gotInt, ok := got.(int64); !ok || gotInt != want {
		t.Errorf("i 期望 %d（退出时刚好等于 threshold），实际 %v (%T)", want, got, got)
	}

	if _, ran := rec.RootFrame.NodeStates["marker"]; !ran {
		t.Errorf("循环结束应触发 done 分支 marker")
	}
}

// TestWhileLoop_FirstFalse i 初始 10 >= threshold 5，condition 首次即 false，body 不执行
func TestWhileLoop_FirstFalse(t *testing.T) {
	rec := runWhileWorkflow(t, buildWhileWorkflow("wf-while-first-false", 10))

	// i 不应被 body 修改
	got := rec.RootFrame.Variables["i"]
	want := int64(10)
	if gotInt, ok := got.(int64); !ok || gotInt != want {
		t.Errorf("i 期望保持初始 %d，实际 %v (%T)", want, got, got)
	}

	if state, ran := rec.RootFrame.NodeStates["set_i"]; ran {
		t.Errorf("condition 首次即 false，body 内 set_i 不应被执行，实际状态 %v", state)
	}

	if _, ran := rec.RootFrame.NodeStates["marker"]; !ran {
		t.Errorf("即使 body 不执行，done 分支 marker 也应触发")
	}
}
