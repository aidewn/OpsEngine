// GraphDiff 单元测试：增/删/改/边变化与摘要文本。
package workflow

import (
	"strings"
	"testing"

	"OpsEngine/internal/core"
)

func node(id, typ string, cfg map[string]any) core.NodeInstance {
	return core.NodeInstance{InstanceID: id, TypeID: typ, Config: cfg}
}

func edge(fn, fp, tn, tp string) core.EdgeConfig {
	return core.EdgeConfig{From: core.PortRef{Node: fn, Port: fp}, To: core.PortRef{Node: tn, Port: tp}}
}

// TestDiffGraph 覆盖四类变更同时发生的场景。
func TestDiffGraph(t *testing.T) {
	old := core.WorkflowDef{
		Nodes: []core.NodeInstance{
			node("a", "system_ready", nil),
			node("b", "linux_exec_command", map[string]any{"command": "ls"}),
			node("c", "print", map[string]any{"message": "hi"}),
		},
		Edges: []core.EdgeConfig{edge("a", "exec_out", "b", "exec_in")},
	}
	new := core.WorkflowDef{
		Nodes: []core.NodeInstance{
			node("a", "system_ready", nil),
			node("b", "linux_exec_command", map[string]any{"command": "ls -la", "timeout": 30}),
			node("d", "var_set", nil),
		},
		Edges: []core.EdgeConfig{edge("a", "exec_out", "d", "exec_in")},
	}

	d := DiffWorkflows(old, new)
	if len(d.Added) != 1 || d.Added[0].InstanceID != "d" {
		t.Fatalf("Added 异常: %#v", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0].InstanceID != "c" {
		t.Fatalf("Removed 异常: %#v", d.Removed)
	}
	if len(d.Changed) != 1 || d.Changed[0].InstanceID != "b" {
		t.Fatalf("Changed 异常: %#v", d.Changed)
	}
	// 变更字段按字典序：command + timeout
	if strings.Join(d.Changed[0].Fields, ",") != "command,timeout" {
		t.Fatalf("Fields 异常: %v", d.Changed[0].Fields)
	}
	if d.EdgesAdded != 1 || d.EdgesRemoved != 1 {
		t.Fatalf("边统计异常: +%d/-%d", d.EdgesAdded, d.EdgesRemoved)
	}

	summary := d.Summary()
	for _, want := range []string{"新增 1 节点", "var_set", "删除 1 节点", "修改 linux_exec_command[b]", "command, timeout", "边 +1/-1"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("摘要缺少 %q: %s", want, summary)
		}
	}
}

// TestDiffGraphEmpty 验证无变更时的摘要。
func TestDiffGraphEmpty(t *testing.T) {
	wf := core.WorkflowDef{Nodes: []core.NodeInstance{node("a", "print", map[string]any{"m": "1"})}}
	d := DiffWorkflows(wf, wf)
	if !d.IsEmpty() || d.Summary() != "无结构变更" {
		t.Fatalf("应为空 diff: %#v %s", d, d.Summary())
	}
}

// TestMaterializeUpdatePreservesIDs 验证更新落地时回显的 instance_id 保留原值。
func TestMaterializeUpdatePreservesIDs(t *testing.T) {
	existing := core.WorkflowDef{
		ID:   "wf-1",
		Name: "旧",
		Nodes: []core.NodeInstance{
			node("keep-1", "system_ready", nil),
			node("update-1", "system_update", map[string]any{"delta_type": "interval", "delta_seconds": 60}),
			node("over-1", "system_over", nil),
		},
	}
	draft := Draft{
		Name: "新",
		Nodes: []DraftNode{
			{ID: "keep-1", TypeID: "system_ready"},                // 回显已有节点
			{ID: "n2", TypeID: "print", Config: map[string]any{}}, // 新增节点
		},
		Edges: []DraftEdge{
			{From: DraftPortRef{Node: "keep-1", Port: "exec_out"}, To: DraftPortRef{Node: "n2", Port: "exec_in"}},
		},
	}
	wf, err := MaterializeWorkflowUpdate(draft, existing, nil)
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if wf.Nodes[0].InstanceID != "keep-1" {
		t.Fatalf("已有节点 ID 应保留: %s", wf.Nodes[0].InstanceID)
	}
	if wf.Nodes[1].InstanceID == "n2" {
		t.Fatalf("新增节点应分配新 UUID，不能用临时 id: %s", wf.Nodes[1].InstanceID)
	}
	// diff 应正确对齐：1 新增 0 删除 0 修改
	d := DiffWorkflows(existing, wf)
	if len(d.Added) != 1 || len(d.Removed) != 0 || len(d.Changed) != 0 {
		t.Fatalf("diff 对齐异常: %#v", d)
	}
}
