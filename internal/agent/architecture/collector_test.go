// 拓扑解析器单测：用真实命令输出样本喂解析器，验证节点/边正确入图。
// 不测真实 SSH 连接（依赖运行环境）；那部分由 CollectFromEnvironment 的集成调用覆盖。

package architecture

import (
	"strings"
	"testing"
)

// TestSplitDiscoverySections 验证 "##X" 标记切段，不漏行不串行。
func TestSplitDiscoverySections(t *testing.T) {
	raw := `##HOSTNAME
web-01
##LISTEN
tcp LISTEN 0 511 0.0.0.0:80 0.0.0.0:* users:(("nginx",pid=1,fd=6))
##DOCKER
##PROCESSES
1234 root 25.0 nginx`
	out := splitDiscoverySections(raw)
	if out["HOSTNAME"] != "web-01" {
		t.Fatalf("HOSTNAME: %q", out["HOSTNAME"])
	}
	if !strings.Contains(out["LISTEN"], "nginx") {
		t.Fatalf("LISTEN: %q", out["LISTEN"])
	}
	if out["DOCKER"] != "" {
		t.Fatalf("空 DOCKER 段应为空字符串，got %q", out["DOCKER"])
	}
	if !strings.Contains(out["PROCESSES"], "1234 root") {
		t.Fatalf("PROCESSES: %q", out["PROCESSES"])
	}
}

// TestParseListensFromSS 验证 ss -tlnp 典型输出能被识别出 port + 进程名。
func TestParseListensFromSS(t *testing.T) {
	raw := `State  Recv-Q Send-Q Local Address:Port Peer Address:Port Process
LISTEN 0      511    0.0.0.0:80         0.0.0.0:*         users:(("nginx",pid=1234,fd=6))
LISTEN 0      4096   127.0.0.1:5432     0.0.0.0:*         users:(("postgres",pid=99,fd=5))
LISTEN 0      100    [::]:443           [::]:*            users:(("nginx",pid=1234,fd=7))`
	// 上面 ss 输出每行实际还应该有 "tcp"/"tcp6" 协议列。补充一下：
	raw = "tcp LISTEN 0 511 0.0.0.0:80 0.0.0.0:* users:((\"nginx\",pid=1234,fd=6))\n" +
		"tcp LISTEN 0 4096 127.0.0.1:5432 0.0.0.0:* users:((\"postgres\",pid=99,fd=5))\n" +
		"tcp6 LISTEN 0 100 [::]:443 [::]:* users:((\"nginx\",pid=1234,fd=7))"

	g := &TopologyGraph{}
	srvID := NewNodeID(NodeKindServer, "cfg-1")
	g.AddNode(TopologyNode{ID: srvID, Kind: NodeKindServer, Label: "web-01"})
	parseListens(g, srvID, "cfg-1", raw)

	if g.CountByKind(NodeKindPort) != 3 {
		t.Fatalf("expected 3 port nodes, got %d: %#v", g.CountByKind(NodeKindPort), g.Nodes)
	}
	// 验证 process 名解析正确
	var foundNginx, foundPostgres bool
	for _, n := range g.Nodes {
		if n.Kind != NodeKindPort {
			continue
		}
		if n.Attrs["process"] == "nginx" {
			foundNginx = true
		}
		if n.Attrs["process"] == "postgres" {
			foundPostgres = true
		}
	}
	if !foundNginx || !foundPostgres {
		t.Fatalf("进程名解析丢失: %#v", g.Nodes)
	}
}

// TestParseListensFromNetstat 验证 netstat -tlnp 输出（pid/name 形式）也能解析。
func TestParseListensFromNetstat(t *testing.T) {
	raw := "tcp 0 0 0.0.0.0:22 0.0.0.0:* LISTEN 1/sshd"
	g := &TopologyGraph{}
	srvID := NewNodeID(NodeKindServer, "cfg-1")
	g.AddNode(TopologyNode{ID: srvID, Kind: NodeKindServer})
	parseListens(g, srvID, "cfg-1", raw)
	if g.CountByKind(NodeKindPort) != 1 {
		t.Fatalf("expected 1 port, got %d", g.CountByKind(NodeKindPort))
	}
	if g.Nodes[len(g.Nodes)-1].Attrs["process"] != "sshd" {
		t.Fatalf("process 未识别: %#v", g.Nodes[len(g.Nodes)-1])
	}
}

// TestParseDocker 验证 "name|image|ports" 形式被正确切分。
func TestParseDocker(t *testing.T) {
	raw := "web-app|nginx:latest|0.0.0.0:80->80/tcp\n" +
		"db|postgres:14|5432/tcp"
	g := &TopologyGraph{}
	srvID := NewNodeID(NodeKindServer, "cfg-1")
	g.AddNode(TopologyNode{ID: srvID, Kind: NodeKindServer})
	parseDocker(g, srvID, "cfg-1", raw)
	if g.CountByKind(NodeKindContainer) != 2 {
		t.Fatalf("expected 2 containers, got %d", g.CountByKind(NodeKindContainer))
	}
	// 验证 image 被存入 attrs
	var foundImage bool
	for _, n := range g.Nodes {
		if n.Kind == NodeKindContainer && n.Attrs["image"] == "postgres:14" {
			foundImage = true
		}
	}
	if !foundImage {
		t.Fatalf("image 未保留: %#v", g.Nodes)
	}
}

// TestParseProcessesSkipsIdle 验证 0% CPU 的进程不会污染图。
func TestParseProcessesSkipsIdle(t *testing.T) {
	raw := "1234 root 25.0 nginx\n5678 root 0.0 kthreadd\n9012 www 12.5 php-fpm"
	g := &TopologyGraph{}
	srvID := NewNodeID(NodeKindServer, "cfg-1")
	g.AddNode(TopologyNode{ID: srvID, Kind: NodeKindServer})
	parseProcesses(g, srvID, "cfg-1", raw)
	if g.CountByKind(NodeKindProcess) != 2 {
		t.Fatalf("expected 2 processes (skip idle), got %d", g.CountByKind(NodeKindProcess))
	}
}
