// AI 草案专用校验单测：连通性（多根）与端口存在性。
package engine

import (
	"strings"
	"testing"

	"OpsEngine/internal/core"
)

func gNode(id, typ string) core.NodeInstance {
	return core.NodeInstance{InstanceID: id, TypeID: typ, Config: map[string]any{}}
}

func gEdge(fn, fp, tn, tp string) core.EdgeConfig {
	return core.EdgeConfig{From: core.PortRef{Node: fn, Port: fp}, To: core.PortRef{Node: tn, Port: tp}}
}

var workflowRoots = []string{"system_ready", "system_update", "system_over"}

// TestConnectivityRejectsOrphan 验证未连线的动作节点被拒绝并指名。
func TestConnectivityRejectsOrphan(t *testing.T) {
	nodes := []core.NodeInstance{
		gNode("r1", "system_ready"),
		gNode("a1", "print"),
		gNode("a2", "print"),
	}
	edges := []core.EdgeConfig{gEdge("r1", "exec_out", "a1", "exec_in")}
	err := ValidateGraphConnectivity(nodes, edges, workflowRoots)
	if err == nil || !strings.Contains(err.Error(), "a2(print)") {
		t.Fatalf("应指名孤立节点 a2: %v", err)
	}
}

// TestConnectivityNoEdges 验证多节点零边的草案被拒绝。
func TestConnectivityNoEdges(t *testing.T) {
	nodes := []core.NodeInstance{
		gNode("r1", "system_ready"),
		gNode("a1", "print"),
	}
	if err := ValidateGraphConnectivity(nodes, nil, workflowRoots); err == nil {
		t.Fatal("零边多节点应被拒绝")
	}
}

// TestConnectivityLifecycleRootsAllowed 验证 system_update/system_over 不连主链是合法的。
func TestConnectivityLifecycleRootsAllowed(t *testing.T) {
	nodes := []core.NodeInstance{
		gNode("r1", "system_ready"),
		gNode("u1", "system_update"),
		gNode("o1", "system_over"),
		gNode("a1", "print"),
	}
	edges := []core.EdgeConfig{gEdge("r1", "exec_out", "a1", "exec_in")}
	if err := ValidateGraphConnectivity(nodes, edges, workflowRoots); err != nil {
		t.Fatalf("生命周期根节点不应被判孤立: %v", err)
	}
}

// TestConnectivityDataSourceViaUndirected 验证纯数据源节点（如 var_get）经数据边按无向连通算连接。
func TestConnectivityDataSourceViaUndirected(t *testing.T) {
	nodes := []core.NodeInstance{
		gNode("r1", "system_ready"),
		gNode("v1", "var_get"),
		gNode("a1", "print"),
	}
	edges := []core.EdgeConfig{
		gEdge("r1", "exec_out", "a1", "exec_in"),
		gEdge("v1", "value", "a1", "message"), // 数据边：var_get -> print
	}
	if err := ValidateGraphConnectivity(nodes, edges, workflowRoots); err != nil {
		t.Fatalf("数据源节点应视为已连接: %v", err)
	}
}

// TestEdgePortsRejectUnknownPort 验证不存在的端口被拒绝并附可用端口列表。
func TestEdgePortsRejectUnknownPort(t *testing.T) {
	nodes := []core.NodeInstance{
		gNode("r1", "system_ready"),
		gNode("a1", "print"),
	}
	edges := []core.EdgeConfig{gEdge("r1", "out", "a1", "exec_in")} // system_ready 没有 "out"
	err := ValidateEdgePorts(nodes, edges)
	if err == nil || !strings.Contains(err.Error(), "exec_out") {
		t.Fatalf("应报端口不存在并列出可用端口: %v", err)
	}
}

// TestEdgePortsSkipAssembleRefs 验证 assemble:* 动态端口跳过校验。
func TestEdgePortsSkipAssembleRefs(t *testing.T) {
	nodes := []core.NodeInstance{
		gNode("r1", "system_ready"),
		gNode("c1", "assemble:abc"),
	}
	edges := []core.EdgeConfig{gEdge("r1", "exec_out", "c1", "whatever_param")}
	if err := ValidateEdgePorts(nodes, edges); err != nil {
		t.Fatalf("assemble 引用端口应跳过: %v", err)
	}
}
