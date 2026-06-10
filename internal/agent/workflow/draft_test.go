// 工作流草案解析与落地的单元测试。

package workflow

import (
	"errors"
	"fmt"
	"strings"
	"testing"

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
	if len(wf.Nodes) != 2 || len(wf.Edges) != 1 {
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
