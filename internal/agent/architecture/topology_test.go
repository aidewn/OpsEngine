// 拓扑数据结构与 Mermaid 渲染单测。

package architecture

import (
	"strings"
	"testing"
)

// TestAddNodeDeduplicates 验证同 ID 不会重复入图。
func TestAddNodeDeduplicates(t *testing.T) {
	g := &TopologyGraph{}
	g.AddNode(TopologyNode{ID: "a", Kind: NodeKindServer, Label: "a"})
	g.AddNode(TopologyNode{ID: "a", Kind: NodeKindServer, Label: "a-dup"})
	if len(g.Nodes) != 1 || g.Nodes[0].Label != "a" {
		t.Fatalf("AddNode 去重失败: %#v", g.Nodes)
	}
}

// TestAddEdgeDeduplicates 验证完全相同的边只入图一次。
func TestAddEdgeDeduplicates(t *testing.T) {
	g := &TopologyGraph{}
	g.AddEdge(TopologyEdge{From: "a", To: "b", Kind: EdgeContains})
	g.AddEdge(TopologyEdge{From: "a", To: "b", Kind: EdgeContains})
	if len(g.Edges) != 1 {
		t.Fatalf("AddEdge 去重失败: %#v", g.Edges)
	}
}

// TestRenderMermaidEmpty 验证空图渲染出可读的占位节点。
func TestRenderMermaidEmpty(t *testing.T) {
	out := RenderMermaid(TopologyGraph{})
	if !strings.Contains(out, "空环境") {
		t.Fatalf("空图占位异常: %s", out)
	}
}

// TestRenderMermaidIncludesAllParts 验证渲染产物结构完整。
func TestRenderMermaidIncludesAllParts(t *testing.T) {
	g := TopologyGraph{EnvironmentID: "e", EnvironmentName: "prod"}
	envID := NewNodeID(NodeKindEnv, "e")
	srvID := NewNodeID(NodeKindServer, "cfg-1")
	portID := NewNodeID(NodeKindPort, "cfg-1", "80", "tcp")
	g.AddNode(TopologyNode{ID: envID, Kind: NodeKindEnv, Label: "prod"})
	g.AddNode(TopologyNode{ID: srvID, Kind: NodeKindServer, Label: "web-01",
		Attrs: map[string]string{"hostname": "web-01.local"}})
	g.AddNode(TopologyNode{ID: portID, Kind: NodeKindPort, Label: "nginx:80"})
	g.AddEdge(TopologyEdge{From: envID, To: srvID, Kind: EdgeContains})
	g.AddEdge(TopologyEdge{From: srvID, To: portID, Kind: EdgeContains})

	out := RenderMermaid(g)
	for _, want := range []string{
		"```mermaid", "graph TD",
		"prod", "web-01", "nginx:80",
		"classDef server",
		"class n", // 短 id 形如 n0/n1
		"-->",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("缺少片段 %q\n%s", want, out)
		}
	}
	// 不应出现冒号原始 NodeID（会被映射成 n0/n1）。
	if strings.Contains(out, "env:e") || strings.Contains(out, "server:cfg-1") {
		t.Fatalf("外部 ID 不应直接出现在 Mermaid 输出: %s", out)
	}
}

// TestEscapeLabelHandlesDangerousChars 验证标签中特殊字符被转义。
func TestEscapeLabelHandlesDangerousChars(t *testing.T) {
	got := escapeLabel(`a "b" <c>`)
	if strings.Contains(got, `"`) || strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Fatalf("escapeLabel 未处理: %s", got)
	}
}

// TestCountByKind 验证 Kind 计数。
func TestCountByKind(t *testing.T) {
	g := TopologyGraph{}
	g.AddNode(TopologyNode{ID: "a", Kind: NodeKindServer})
	g.AddNode(TopologyNode{ID: "b", Kind: NodeKindServer})
	g.AddNode(TopologyNode{ID: "c", Kind: NodeKindContainer})
	if g.CountByKind(NodeKindServer) != 2 || g.CountByKind(NodeKindContainer) != 1 {
		t.Fatalf("CountByKind 异常")
	}
}
