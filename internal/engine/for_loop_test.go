// Phase 8 Step 3：for_loop 节点计数循环测试
//
// 验证：
//   - 正常迭代 [0,5) 配合 var_set + math_add + index 累加 sum=10
//   - 空区间 [3,3) 不进入 body
//   - 逆向 [5,0) 不进入 body（start>=end）
//   - 循环结束后 exec_out_done 被触发

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

// runForWorkflow 落盘工作流并运行到结束，返回执行记录
func runForWorkflow(t *testing.T, wf core.WorkflowDef) core.ExecutionRecord {
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
	return rt.Record()
}

// TestForLoop_AccumulatesSum 累加测试：[0,5) 每次 sum += index，最终 sum=10
//
// 工作流结构：
//
//	ready → for(start=0, end=5) → body → set(sum, add(get(sum), for.index))
//	                            → done → done_marker(print)
func TestForLoop_AccumulatesSum(t *testing.T) {
	wf := core.WorkflowDef{
		ID: "wf-for-sum",
		Variables: []core.VariableDef{
			{Name: "sum", VarType: core.PortTypeInt, Default: int64(0)},
			{Name: "loop_start", VarType: core.PortTypeInt, Default: int64(0)},
			{Name: "loop_end", VarType: core.PortTypeInt, Default: int64(5)},
			{Name: "msg", VarType: core.PortTypeString, Default: "done"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready"},
			{InstanceID: "fl", TypeID: "for_loop"},
			{InstanceID: "set_sum", TypeID: "var_set",
				Config: map[string]any{"var_name": "sum", "var_type": "Int"}},
			{InstanceID: "add", TypeID: "math_add",
				Config: map[string]any{"var_type": "Int"}},
			{InstanceID: "get_sum", TypeID: "var_get",
				Config: map[string]any{"var_name": "sum", "var_type": "Int"}},
			{InstanceID: "get_start", TypeID: "var_get",
				Config: map[string]any{"var_name": "loop_start", "var_type": "Int"}},
			{InstanceID: "get_end", TypeID: "var_get",
				Config: map[string]any{"var_name": "loop_end", "var_type": "Int"}},
			{InstanceID: "marker", TypeID: "print"},
			{InstanceID: "get_msg", TypeID: "var_get",
				Config: map[string]any{"var_name": "msg", "var_type": "String"}},
		},
		Edges: []core.EdgeConfig{
			// 主流
			{From: core.PortRef{Node: "ready", Port: "exec_out"}, To: core.PortRef{Node: "fl", Port: "exec_in"}},
			{From: core.PortRef{Node: "fl", Port: "exec_out_body"}, To: core.PortRef{Node: "set_sum", Port: "exec_in"}},
			{From: core.PortRef{Node: "fl", Port: "exec_out_done"}, To: core.PortRef{Node: "marker", Port: "exec_in"}},
			// for_loop 输入
			{From: core.PortRef{Node: "get_start", Port: "value"}, To: core.PortRef{Node: "fl", Port: "start"}},
			{From: core.PortRef{Node: "get_end", Port: "value"}, To: core.PortRef{Node: "fl", Port: "end"}},
			// add 的两个加数：a = sum 当前值, b = for.index
			{From: core.PortRef{Node: "get_sum", Port: "value"}, To: core.PortRef{Node: "add", Port: "a"}},
			{From: core.PortRef{Node: "fl", Port: "index"}, To: core.PortRef{Node: "add", Port: "b"}},
			// set_sum.value = add.result
			{From: core.PortRef{Node: "add", Port: "result"}, To: core.PortRef{Node: "set_sum", Port: "value"}},
			// done 分支日志
			{From: core.PortRef{Node: "get_msg", Port: "value"}, To: core.PortRef{Node: "marker", Port: "message"}},
		},
	}

	rec := runForWorkflow(t, wf)

	got := rec.RootFrame.Variables["sum"]
	want := int64(0 + 1 + 2 + 3 + 4) // = 10
	if gotInt, ok := got.(int64); !ok || gotInt != want {
		t.Errorf("sum 期望 %d，实际 %v (%T)", want, got, got)
	}

	// done 分支确实被触发
	if _, ran := rec.RootFrame.NodeStates["marker"]; !ran {
		t.Errorf("exec_out_done 应触发 marker，实际未执行")
	}
}

// TestForLoop_EmptyRange [3,3) 不进入 body，但 done 仍然走
func TestForLoop_EmptyRange(t *testing.T) {
	wf := buildEmptyOrReverseLoop("wf-for-empty", int64(3), int64(3))
	rec := runForWorkflow(t, wf)

	if state, ran := rec.RootFrame.NodeStates["body_marker"]; ran {
		t.Errorf("空区间不应执行 body，实际状态 %v", state)
	}
	if _, ran := rec.RootFrame.NodeStates["done_marker"]; !ran {
		t.Errorf("空区间也应走 done 分支")
	}
}

// TestForLoop_ReverseRange [5,0) start>=end，不进入 body
func TestForLoop_ReverseRange(t *testing.T) {
	wf := buildEmptyOrReverseLoop("wf-for-reverse", int64(5), int64(0))
	rec := runForWorkflow(t, wf)

	if state, ran := rec.RootFrame.NodeStates["body_marker"]; ran {
		t.Errorf("逆向区间不应执行 body，实际状态 %v", state)
	}
	if _, ran := rec.RootFrame.NodeStates["done_marker"]; !ran {
		t.Errorf("逆向区间也应走 done 分支")
	}
}

// buildEmptyOrReverseLoop 构造一个只验证"是否进 body / 是否走 done"的最小工作流
//
//	ready → for(start, end) → body → body_marker(print)
//	                        → done → done_marker(print)
func buildEmptyOrReverseLoop(id string, start, end int64) core.WorkflowDef {
	return core.WorkflowDef{
		ID: id,
		Variables: []core.VariableDef{
			{Name: "loop_start", VarType: core.PortTypeInt, Default: start},
			{Name: "loop_end", VarType: core.PortTypeInt, Default: end},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready"},
			{InstanceID: "fl", TypeID: "for_loop"},
			{InstanceID: "body_marker", TypeID: "print"},
			{InstanceID: "done_marker", TypeID: "print"},
			{InstanceID: "get_start", TypeID: "var_get",
				Config: map[string]any{"var_name": "loop_start", "var_type": "Int"}},
			{InstanceID: "get_end", TypeID: "var_get",
				Config: map[string]any{"var_name": "loop_end", "var_type": "Int"}},
		},
		Edges: []core.EdgeConfig{
			{From: core.PortRef{Node: "ready", Port: "exec_out"}, To: core.PortRef{Node: "fl", Port: "exec_in"}},
			{From: core.PortRef{Node: "fl", Port: "exec_out_body"}, To: core.PortRef{Node: "body_marker", Port: "exec_in"}},
			{From: core.PortRef{Node: "fl", Port: "exec_out_done"}, To: core.PortRef{Node: "done_marker", Port: "exec_in"}},
			{From: core.PortRef{Node: "get_start", Port: "value"}, To: core.PortRef{Node: "fl", Port: "start"}},
			{From: core.PortRef{Node: "get_end", Port: "value"}, To: core.PortRef{Node: "fl", Port: "end"}},
		},
	}
}
