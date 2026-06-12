package builtin

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"OpsEngine/internal/agent/tools"
)

// TestRenderDiagramValid 验证合法 spec 产出 diagram 视图。
func TestRenderDiagramValid(t *testing.T) {
	spec := `{"title":"根因","nodes":[{"id":"f1","label":"磁盘满","kind":"fact"},{"id":"j1","label":"日志未轮转","kind":"judgment"}],"edges":[{"from":"f1","to":"j1"}]}`
	res, err := RenderDiagram{}.Execute(tools.ToolContext{}, map[string]any{"spec": spec})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.View == nil || res.View.Kind != "diagram" || res.View.Title != "根因" {
		t.Fatalf("视图异常: %#v", res.View)
	}
	var got diagramSpec
	if err := json.Unmarshal([]byte(res.View.Data), &got); err != nil {
		t.Fatalf("Data 非法 JSON: %v", err)
	}
	if len(got.Nodes) != 2 || len(got.Edges) != 1 {
		t.Fatalf("规范化结果异常: %#v", got)
	}
}

// TestRenderDiagramRejectsDanglingEdge 验证悬空边被拒绝。
func TestRenderDiagramRejectsDanglingEdge(t *testing.T) {
	spec := `{"nodes":[{"id":"a","label":"A"}],"edges":[{"from":"a","to":"missing"}]}`
	_, err := RenderDiagram{}.Execute(tools.ToolContext{}, map[string]any{"spec": spec})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("应拒绝悬空边: %v", err)
	}
}

// TestRenderDiagramRejectsTooMany 验证超过节点上限被拒绝。
func TestRenderDiagramRejectsTooMany(t *testing.T) {
	var nodes []string
	for i := 0; i < maxDiagramNodes+1; i++ {
		nodes = append(nodes, `{"id":"n`+string(rune('A'+i%26))+strconv.Itoa(i)+`","label":"x"}`)
	}
	spec := `{"nodes":[` + strings.Join(nodes, ",") + `]}`
	_, err := RenderDiagram{}.Execute(tools.ToolContext{}, map[string]any{"spec": spec})
	if err == nil || !strings.Contains(err.Error(), "上限") {
		t.Fatalf("应拒绝超量节点: %v", err)
	}
}
