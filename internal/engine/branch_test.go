// Phase 8 Step 2：branch 节点条件分支测试
// 验证：condition=true / false / 未连接三种情况下分别走的分支

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

// buildBranchWorkflow 构造一个 ready → branch → (true) pt / (false) pf 的工作流
//   - condDefault: 变量 cond 的初始值
//   - condConnected: 是否把 var_get(cond) 连到 branch.condition
func buildBranchWorkflow(condDefault any, condConnected bool) core.WorkflowDef {
	wf := core.WorkflowDef{
		ID: "wf-branch",
		Variables: []core.VariableDef{
			{Name: "cond", VarType: core.PortTypeBool, Default: condDefault},
			{Name: "msg_t", VarType: core.PortTypeString, Default: "TRUE"},
			{Name: "msg_f", VarType: core.PortTypeString, Default: "FALSE"},
		},
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready"},
			{InstanceID: "br", TypeID: "branch"},
			{InstanceID: "pt", TypeID: "print"},
			{InstanceID: "pf", TypeID: "print"},
			{InstanceID: "gc", TypeID: "var_get", Config: map[string]any{"var_name": "cond", "var_type": "Bool"}},
			{InstanceID: "gt", TypeID: "var_get", Config: map[string]any{"var_name": "msg_t", "var_type": "String"}},
			{InstanceID: "gf", TypeID: "var_get", Config: map[string]any{"var_name": "msg_f", "var_type": "String"}},
		},
		Edges: []core.EdgeConfig{
			{From: core.PortRef{Node: "ready", Port: "exec_out"}, To: core.PortRef{Node: "br", Port: "exec_in"}},
			{From: core.PortRef{Node: "br", Port: "exec_out_true"}, To: core.PortRef{Node: "pt", Port: "exec_in"}},
			{From: core.PortRef{Node: "br", Port: "exec_out_false"}, To: core.PortRef{Node: "pf", Port: "exec_in"}},
			{From: core.PortRef{Node: "gt", Port: "value"}, To: core.PortRef{Node: "pt", Port: "message"}},
			{From: core.PortRef{Node: "gf", Port: "value"}, To: core.PortRef{Node: "pf", Port: "message"}},
		},
	}
	if condConnected {
		wf.Edges = append(wf.Edges, core.EdgeConfig{
			From: core.PortRef{Node: "gc", Port: "value"},
			To:   core.PortRef{Node: "br", Port: "condition"},
		})
	}
	return wf
}

// runBranchWorkflow 落盘 + Run + 等结束 + 返回执行记录；状态非 Success 时直接 Fatal
func runBranchWorkflow(t *testing.T, wf core.WorkflowDef) core.ExecutionRecord {
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

// TestBranch_True condition=true 走 true 分支，false 分支节点不被执行
func TestBranch_True(t *testing.T) {
	rec := runBranchWorkflow(t, buildBranchWorkflow(true, true))
	if _, ran := rec.RootFrame.NodeStates["pt"]; !ran {
		t.Errorf("true 分支 pt 应被执行，实际未执行")
	}
	if state, ran := rec.RootFrame.NodeStates["pf"]; ran {
		t.Errorf("false 分支 pf 不应被执行，实际状态 %v", state)
	}
}

// TestBranch_False condition=false 走 false 分支，true 分支节点不被执行
func TestBranch_False(t *testing.T) {
	rec := runBranchWorkflow(t, buildBranchWorkflow(false, true))
	if _, ran := rec.RootFrame.NodeStates["pf"]; !ran {
		t.Errorf("false 分支 pf 应被执行，实际未执行")
	}
	if state, ran := rec.RootFrame.NodeStates["pt"]; ran {
		t.Errorf("true 分支 pt 不应被执行，实际状态 %v", state)
	}
}

// TestBranch_Disconnected condition 未连接，按 false 处理（容错策略）
// 即使变量 cond=true，未连线 condition 端口也应走 false 分支
func TestBranch_Disconnected(t *testing.T) {
	rec := runBranchWorkflow(t, buildBranchWorkflow(true, false))
	if _, ran := rec.RootFrame.NodeStates["pf"]; !ran {
		t.Errorf("condition 未连接应按 false 处理，pf 应被执行")
	}
	if state, ran := rec.RootFrame.NodeStates["pt"]; ran {
		t.Errorf("condition 未连接时 true 分支 pt 不应被执行，实际状态 %v", state)
	}
}
