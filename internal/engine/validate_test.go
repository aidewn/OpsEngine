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
