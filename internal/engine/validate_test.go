// validate.go 的单元测试

package engine

import (
	"strings"
	"testing"

	"OpsEngine/internal/core"
)

// TestValidateWorkflow_Singletons 验证单例约束
func TestValidateWorkflow_Singletons(t *testing.T) {
	// 2 个 system_ready → 失败
	wf := core.WorkflowDef{
		Nodes: []core.NodeInstance{
			{InstanceID: "n1", TypeID: "system_ready"},
			{InstanceID: "n2", TypeID: "system_ready"},
		},
	}
	err := ValidateWorkflow(wf)
	if err == nil {
		t.Fatal("期望失败，但通过了校验")
	}
	if !strings.Contains(err.Error(), "system_ready") {
		t.Fatalf("错误信息应该提到 system_ready，实际: %v", err)
	}

	// 1 个 system_ready + 1 个 system_update → 通过
	wf.Nodes = []core.NodeInstance{
		{InstanceID: "n1", TypeID: "system_ready"},
		{InstanceID: "n2", TypeID: "system_update"},
		{InstanceID: "n3", TypeID: "print"},
		{InstanceID: "n4", TypeID: "print"},
	}
	if err := ValidateWorkflow(wf); err != nil {
		t.Fatalf("期望通过，实际失败: %v", err)
	}
}

// TestValidateWorkflow_ExecOutSingle 验证 exec_out 单连接
func TestValidateWorkflow_ExecOutSingle(t *testing.T) {
	wf := core.WorkflowDef{
		Edges: []core.EdgeConfig{
			// n1.exec_out → n2.exec_in
			{From: core.PortRef{Node: "n1", Port: "exec_out"}, To: core.PortRef{Node: "n2", Port: "exec_in"}},
			// n1.exec_out → n3.exec_in（第二条出边，应该被拒绝）
			{From: core.PortRef{Node: "n1", Port: "exec_out"}, To: core.PortRef{Node: "n3", Port: "exec_in"}},
		},
	}
	err := ValidateWorkflow(wf)
	if err == nil {
		t.Fatal("期望失败，但通过了校验")
	}
	if !strings.Contains(err.Error(), "exec") {
		t.Fatalf("错误信息应该提到 exec，实际: %v", err)
	}

	// 数据端口允许多边
	wf.Edges = []core.EdgeConfig{
		{From: core.PortRef{Node: "n1", Port: "value"}, To: core.PortRef{Node: "n2", Port: "message"}},
		{From: core.PortRef{Node: "n1", Port: "value"}, To: core.PortRef{Node: "n3", Port: "message"}},
	}
	if err := ValidateWorkflow(wf); err != nil {
		t.Fatalf("数据端口多边应通过，实际失败: %v", err)
	}
}

// TestValidateWorkflow_InputSingle 验证 input 端口单连接
func TestValidateWorkflow_InputSingle(t *testing.T) {
	// 同一 input 端口被 2 条边连入 → 失败
	wf := core.WorkflowDef{
		Edges: []core.EdgeConfig{
			{From: core.PortRef{Node: "n1", Port: "value"}, To: core.PortRef{Node: "n3", Port: "message"}},
			{From: core.PortRef{Node: "n2", Port: "value"}, To: core.PortRef{Node: "n3", Port: "message"}},
		},
	}
	if err := ValidateWorkflow(wf); err == nil {
		t.Fatal("期望失败：同一 input 接收多条边")
	} else if !strings.Contains(err.Error(), "message") {
		t.Fatalf("错误信息应提到端口名，实际: %v", err)
	}

	// exec_in 也必须单入
	wf.Edges = []core.EdgeConfig{
		{From: core.PortRef{Node: "n1", Port: "exec_out"}, To: core.PortRef{Node: "n3", Port: "exec_in"}},
		{From: core.PortRef{Node: "n2", Port: "exec_out"}, To: core.PortRef{Node: "n3", Port: "exec_in"}},
	}
	if err := ValidateWorkflow(wf); err == nil {
		t.Fatal("期望失败：exec_in 接收多条边")
	}

	// output 一对多仍然允许
	wf.Edges = []core.EdgeConfig{
		{From: core.PortRef{Node: "n1", Port: "value"}, To: core.PortRef{Node: "n2", Port: "message"}},
		{From: core.PortRef{Node: "n1", Port: "value"}, To: core.PortRef{Node: "n3", Port: "message"}},
	}
	if err := ValidateWorkflow(wf); err != nil {
		t.Fatalf("output 一对多应允许，实际失败: %v", err)
	}
}

// TestValidateAssemble_Singletons 集合的单例约束
func TestValidateAssemble_Singletons(t *testing.T) {
	asm := core.AssembleDef{
		Nodes: []core.NodeInstance{
			{InstanceID: "n1", TypeID: "assemble_start"},
			{InstanceID: "n2", TypeID: "assemble_start"},
		},
	}
	if err := ValidateAssemble(asm); err == nil {
		t.Fatal("期望失败，但通过了校验")
	}
}

// TestValidateWorkflow_VariableRefs var_set / var_get 引用的变量必须存在
func TestValidateWorkflow_VariableRefs(t *testing.T) {
	// 缺 var_name
	wf := core.WorkflowDef{
		Nodes: []core.NodeInstance{
			{InstanceID: "n1", TypeID: "var_set", Config: map[string]any{}},
		},
	}
	if err := ValidateWorkflow(wf); err == nil ||
		!strings.Contains(err.Error(), "var_name 未配置") {
		t.Fatalf("期望 var_name 未配置错误，实际: %v", err)
	}

	// 引用未定义变量
	wf = core.WorkflowDef{
		Nodes: []core.NodeInstance{
			{InstanceID: "n1", TypeID: "var_get",
				Config: map[string]any{"var_name": "ghost"}},
		},
	}
	if err := ValidateWorkflow(wf); err == nil ||
		!strings.Contains(err.Error(), "ghost") {
		t.Fatalf("期望 ghost 未定义错误，实际: %v", err)
	}

	// 引用已定义变量 → 通过
	wf = core.WorkflowDef{
		Variables: []core.VariableDef{{Name: "count", VarType: core.PortTypeInt}},
		Nodes: []core.NodeInstance{
			{InstanceID: "n1", TypeID: "var_set",
				Config: map[string]any{"var_name": "count"}},
			{InstanceID: "n2", TypeID: "var_get",
				Config: map[string]any{"var_name": "count"}},
		},
	}
	if err := ValidateWorkflow(wf); err != nil {
		t.Fatalf("期望通过，实际失败: %v", err)
	}
}

// TestValidateAssemble_VariableRefs 集合也走同一套校验
func TestValidateAssemble_VariableRefs(t *testing.T) {
	asm := core.AssembleDef{
		Nodes: []core.NodeInstance{
			{InstanceID: "n1", TypeID: "var_set",
				Config: map[string]any{"var_name": "missing"}},
		},
	}
	if err := ValidateAssemble(asm); err == nil ||
		!strings.Contains(err.Error(), "missing") {
		t.Fatalf("期望 missing 未定义错误，实际: %v", err)
	}
}

// TestValidateAssemble_ParamReturnRefs assemble_param / return_set 引用必须存在
func TestValidateAssemble_ParamReturnRefs(t *testing.T) {
	asm := core.AssembleDef{
		Params:  []core.ParamDef{{Name: "in", VarType: core.PortTypeString}},
		Returns: []core.ParamDef{{Name: "out", VarType: core.PortTypeString}},
		Nodes: []core.NodeInstance{
			{InstanceID: "p1", TypeID: "assemble_param",
				Config: map[string]any{"param_name": "ghost"}},
			{InstanceID: "r1", TypeID: "return_set",
				Config: map[string]any{"return_name": "ghost"}},
		},
	}
	if err := ValidateAssemble(asm); err == nil ||
		!strings.Contains(err.Error(), "ghost") {
		t.Fatalf("期望 ghost 未定义错误，实际: %v", err)
	}

	asm.Nodes = []core.NodeInstance{
		{InstanceID: "p1", TypeID: "assemble_param",
			Config: map[string]any{"param_name": "in"}},
		{InstanceID: "r1", TypeID: "return_set",
			Config: map[string]any{"return_name": "out"}},
	}
	if err := ValidateAssemble(asm); err != nil {
		t.Fatalf("期望通过，实际失败: %v", err)
	}
}
