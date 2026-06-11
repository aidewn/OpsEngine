// 工作流草案解析与落地的单元测试。

package workflow

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	_ "OpsEngine/internal/nodes" // 触发节点注册，让 engine.Lookup 在测试中可用
)

// TestParseDraft 验证可以从模型回复中提取 JSON 草案，并兼容 Markdown 围栏。
func TestParseDraft(t *testing.T) {
	reply := "好的，这是工作流：\n```json\n{\"name\":\"打印\",\"description\":\"\",\"variables\":[],\"nodes\":[{\"id\":\"n1\",\"type_id\":\"system_ready\",\"config\":{},\"position\":{\"x\":80,\"y\":120}}],\"edges\":[],\"notes\":[]}\n```"
	draft, err := ParseDraft(reply)
	if err != nil {
		t.Fatalf("ParseDraft() error = %v", err)
	}
	if draft.Name != "打印" {
		t.Fatalf("Name = %q", draft.Name)
	}
	if len(draft.Nodes) != 1 || draft.Nodes[0].TypeID != "system_ready" {
		t.Fatalf("Nodes = %#v", draft.Nodes)
	}
}

// TestParseDraftEmptyNodes 验证空节点列表会被拒绝。
func TestParseDraftEmptyNodes(t *testing.T) {
	if _, err := ParseDraft(`{"name":"x","nodes":[]}`); err == nil {
		t.Fatal("expected error for empty nodes")
	}
}

// realChecker 通过 engine.Lookup 校验类型存在性。生产代码也会再叠加 assemble 校验。
func realChecker(typeID string) error {
	if strings.TrimSpace(typeID) == "" {
		return errors.New("节点缺少 type_id")
	}
	if _, ok := engine.Lookup(typeID); !ok {
		return fmt.Errorf("未知节点类型: %s", typeID)
	}
	return nil
}

// TestMaterialize 验证 id 重写、边映射、工作流校验全部走通。
func TestMaterialize(t *testing.T) {
	draft := Draft{
		Name:        "最小工作流",
		Description: "system_ready -> print",
		Nodes: []DraftNode{
			{ID: "n1", TypeID: "system_ready", Position: DraftPosition{X: 80, Y: 120}},
			{ID: "n2", TypeID: "print", Config: map[string]any{"text": "hello"}, Position: DraftPosition{X: 360, Y: 120}},
		},
		Edges: []DraftEdge{
			{From: DraftPortRef{Node: "n1", Port: "exec_out"}, To: DraftPortRef{Node: "n2", Port: "exec_in"}},
		},
	}
	wf, err := Materialize(draft, realChecker)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if len(wf.Nodes) != 4 || len(wf.Edges) != 1 {
		t.Fatalf("nodes/edges = %d/%d", len(wf.Nodes), len(wf.Edges))
	}
	for _, node := range wf.Nodes {
		if node.InstanceID == "n1" || node.InstanceID == "n2" {
			t.Fatalf("临时 id 未重写: %s", node.InstanceID)
		}
	}
	edge := wf.Edges[0]
	if edge.From.Node == "n1" || edge.To.Node == "n2" {
		t.Fatalf("边未跟随 id 重写: %#v", edge)
	}
}

// TestMaterializeAddsMissingLifecycleNodes 验证 AI 新建工作流时会补齐默认生命周期节点。
func TestMaterializeAddsMissingLifecycleNodes(t *testing.T) {
	wf, err := Materialize(Draft{
		Name:  "只给入口",
		Nodes: []DraftNode{{ID: "n1", TypeID: "system_ready"}},
	}, realChecker)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	for _, typeID := range []string{"system_ready", "system_update", "system_over"} {
		if countNodesByType(wf.Nodes, typeID) != 1 {
			t.Fatalf("生命周期节点 %s 应该存在且仅存在 1 个: %#v", typeID, wf.Nodes)
		}
	}
	update := findNodeByType(wf.Nodes, "system_update")
	if update.Config["delta_type"] != "interval" || update.Config["delta_seconds"] != 60 {
		t.Fatalf("system_update 默认配置异常: %#v", update.Config)
	}
}

// TestMaterializeWorkflowUpdatePreservesExistingLifecycleNodes 验证 AI 更新不会误删已有生命周期节点。
func TestMaterializeWorkflowUpdatePreservesExistingLifecycleNodes(t *testing.T) {
	existing := core.WorkflowDef{
		ID:   "wf-1",
		Name: "已有工作流",
		Nodes: []core.NodeInstance{
			{InstanceID: "ready-1", TypeID: "system_ready", Config: map[string]any{}, Position: core.Position{X: 10, Y: 20}},
			{InstanceID: "update-1", TypeID: "system_update", Config: map[string]any{"delta_type": "manual"}, Position: core.Position{X: 30, Y: 40}},
			{InstanceID: "over-1", TypeID: "system_over", Config: map[string]any{}, Position: core.Position{X: 50, Y: 60}},
		},
	}
	wf, err := MaterializeWorkflowUpdate(Draft{
		Name:  "已有工作流",
		Nodes: []DraftNode{{ID: "ready-1", TypeID: "system_ready"}},
	}, existing, realChecker)
	if err != nil {
		t.Fatalf("MaterializeWorkflowUpdate() error = %v", err)
	}
	if findNodeByType(wf.Nodes, "system_update").InstanceID != "update-1" {
		t.Fatalf("system_update 应保留已有节点: %#v", wf.Nodes)
	}
	if findNodeByType(wf.Nodes, "system_update").Config["delta_type"] != "manual" {
		t.Fatalf("system_update 配置应保留: %#v", findNodeByType(wf.Nodes, "system_update").Config)
	}
	if findNodeByType(wf.Nodes, "system_over").InstanceID != "over-1" {
		t.Fatalf("system_over 应保留已有节点: %#v", wf.Nodes)
	}
}

// TestMaterializeRejectsUnknownType 验证未知节点类型会被拒绝。
func TestMaterializeRejectsUnknownType(t *testing.T) {
	_, err := Materialize(Draft{
		Nodes: []DraftNode{{ID: "n1", TypeID: "not_a_real_type"}},
	}, realChecker)
	if err == nil || !strings.Contains(err.Error(), "未知节点类型") {
		t.Fatalf("expected unknown type error, got %v", err)
	}
}

// TestMaterializeRejectsDanglingEdge 验证边引用不存在的节点会被拒绝。
func TestMaterializeRejectsDanglingEdge(t *testing.T) {
	_, err := Materialize(Draft{
		Nodes: []DraftNode{{ID: "n1", TypeID: "system_ready"}},
		Edges: []DraftEdge{{From: DraftPortRef{Node: "n1", Port: "exec_out"}, To: DraftPortRef{Node: "ghost", Port: "exec_in"}}},
	}, realChecker)
	if err == nil || !strings.Contains(err.Error(), "未知节点") {
		t.Fatalf("expected dangling edge error, got %v", err)
	}
}

// TestParseDraftAcceptsInstanceID 验证模型回显 instance_id 时能被 normalize 成临时 id。
func TestParseDraftAcceptsInstanceID(t *testing.T) {
	reply := `{"name":"更新","nodes":[
		{"instance_id":"abc-123","type_id":"system_ready","config":{},"position":{"x":0,"y":0}},
		{"instance_id":"def-456","type_id":"print","config":{"text":"hi"},"position":{"x":100,"y":0}}
	],"edges":[{"from":{"node":"abc-123","port":"exec_out"},"to":{"node":"def-456","port":"exec_in"}}]}`
	draft, err := ParseDraft(reply)
	if err != nil {
		t.Fatalf("ParseDraft() error = %v", err)
	}
	if draft.Nodes[0].ID != "abc-123" {
		t.Fatalf("node0 id = %q, want abc-123", draft.Nodes[0].ID)
	}
	wf, err := Materialize(draft, realChecker)
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if len(wf.Edges) != 1 {
		t.Fatalf("edges = %d", len(wf.Edges))
	}
}

// TestParseDraftAutoAssignsMissingID 验证缺失 id 的节点会自动分配 n1/n2。
func TestParseDraftAutoAssignsMissingID(t *testing.T) {
	reply := `{"name":"新建","nodes":[
		{"type_id":"system_ready","config":{},"position":{"x":0,"y":0}},
		{"type_id":"print","config":{"text":"x"},"position":{"x":100,"y":0}}
	],"edges":[{"from":{"node":"n1","port":"exec_out"},"to":{"node":"n2","port":"exec_in"}}]}`
	draft, err := ParseDraft(reply)
	if err != nil {
		t.Fatalf("ParseDraft() error = %v", err)
	}
	if draft.Nodes[0].ID != "n1" || draft.Nodes[1].ID != "n2" {
		t.Fatalf("auto ids = %q, %q", draft.Nodes[0].ID, draft.Nodes[1].ID)
	}
}

// countNodesByType 统计指定类型节点数量。
func countNodesByType(nodes []core.NodeInstance, typeID string) int {
	count := 0
	for _, node := range nodes {
		if node.TypeID == typeID {
			count++
		}
	}
	return count
}

// findNodeByType 查找指定类型节点；测试失败时返回零值让断言给出完整上下文。
func findNodeByType(nodes []core.NodeInstance, typeID string) core.NodeInstance {
	for _, node := range nodes {
		if node.TypeID == typeID {
			return node
		}
	}
	return core.NodeInstance{}
}
