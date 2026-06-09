// 从环境内所有 SSH 配置采集拓扑事实。
//
// 流程（per SSH 配置）：
//  1. clients.ParseLinuxSshDialConfig → 拨号
//  2. 运行 discovery 脚本，按 "##SECTION" 切段
//  3. 解析 hostname / listen / docker / processes 四段，写入 TopologyGraph
//
// 单台采集失败不致命：记录到 graph.CollectionErrors 并继续下一台。
// SSH 命令超时硬上限 sshCollectTimeout 避免拖死整批。

package architecture

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
)

// sshCollectTimeout 是单台 SSH 采集脚本的硬超时。
const sshCollectTimeout = 20 * time.Second

// EnvironmentLookup 是按 id 取环境定义的回调；与 context/snapshot.go 一致的形态，
// 让 architecture 包不直接依赖 store 层。
type EnvironmentLookup func(environmentID string) (core.EnvironmentDef, error)

// ProgressFunc 是采集进度回调，runtime 据此把每台机的采集状态推到前端。
// 实参：人类可读的进度文本，例如 "正在采集 web-01 (1/3)"。
type ProgressFunc func(text string)

// CollectFromEnvironment 加载环境，逐一拨号其内的 SSH 配置，聚合拓扑。
//   - envLookup 用于解析 environmentID（与 ai.go 注入的 lookup 一致）
//   - onProgress 可为 nil
func CollectFromEnvironment(envLookup EnvironmentLookup, environmentID string, onProgress ProgressFunc) (TopologyGraph, error) {
	if envLookup == nil {
		return TopologyGraph{}, fmt.Errorf("environment lookup 未注入")
	}
	env, err := envLookup(environmentID)
	if err != nil {
		return TopologyGraph{}, err
	}
	g := TopologyGraph{
		EnvironmentID:   env.ID,
		EnvironmentName: env.Name,
	}
	envNodeID := NewNodeID(NodeKindEnv, env.ID)
	g.AddNode(TopologyNode{
		ID: envNodeID, Kind: NodeKindEnv,
		Label: displayName(env.Name, env.ID),
	})

	// 只采集 SSH 配置；其他 kind（Docker/K8s/Jenkins）作为 inventory 已在外层 prompt 中展示，
	// v1 不直连采集（依赖 docker/k8s 客户端，单独切片）。
	sshConfigs := []core.EnvConfigItem{}
	for _, c := range env.Configs {
		if c.Kind == core.EnvConfigKindSSH {
			sshConfigs = append(sshConfigs, c)
		}
	}

	if len(sshConfigs) == 0 {
		// 环境内没有 SSH 也允许出图：env 节点 + inventory 段在外层处理。
		return g, nil
	}

	for i, cfg := range sshConfigs {
		if onProgress != nil {
			onProgress(fmt.Sprintf("正在采集 %s (%d/%d)", displayName(cfg.Name, cfg.ID), i+1, len(sshConfigs)))
		}
		if err := collectOneServer(&g, envNodeID, cfg); err != nil {
			// 非致命：记录到 CollectionErrors 让报告里显式提示
			g.CollectionErrors = append(g.CollectionErrors,
				fmt.Sprintf("%s: %s", displayName(cfg.Name, cfg.ID), err.Error()))
		}
	}
	return g, nil
}

// collectOneServer 拨号一台 SSH 并把结果写入 graph。
func collectOneServer(g *TopologyGraph, envNodeID string, cfg core.EnvConfigItem) error {
	dial, err := clients.ParseLinuxSshDialConfig(cfg.Fields)
	if err != nil {
		return err
	}
	client, err := dial.Dial()
	if err != nil {
		return err
	}
	defer client.Close()

	session, err := client.Client().NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := session.CombinedOutput(discoveryScript)
		done <- result{out, err}
	}()
	var raw []byte
	select {
	case r := <-done:
		// CombinedOutput 非零退出不致命：很多子命令在缺工具时返回 1，但其他段仍可解析。
		raw = r.out
	case <-time.After(sshCollectTimeout):
		_ = session.Close()
		return fmt.Errorf("采集超时（%s）", sshCollectTimeout)
	}

	// 写主机节点（即使后续解析失败也保留 env→server 的可见性）
	serverID := NewNodeID(NodeKindServer, cfg.ID)
	g.AddNode(TopologyNode{
		ID: serverID, Kind: NodeKindServer,
		Label: displayName(cfg.Name, cfg.ID),
		Attrs: map[string]string{},
	})
	g.AddEdge(TopologyEdge{From: envNodeID, To: serverID, Kind: EdgeContains})

	sections := splitDiscoverySections(string(raw))
	parseHostname(g, serverID, sections["HOSTNAME"])
	parseListens(g, serverID, cfg.ID, sections["LISTEN"])
	parseDocker(g, serverID, cfg.ID, sections["DOCKER"])
	parseProcesses(g, serverID, cfg.ID, sections["PROCESSES"])
	return nil
}

// discoveryScript 是单台 SSH 拓扑采集脚本。
// 用 "##XXX" 作为节标记，便于后端按节切分。
// 每个命令都 2>/dev/null 兜底，缺工具时段为空但节标记仍在。
const discoveryScript = `set +e
echo "##HOSTNAME"
hostname 2>/dev/null
echo "##LISTEN"
(ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null) | head -80
echo "##DOCKER"
docker ps --format '{{.Names}}|{{.Image}}|{{.Ports}}' 2>/dev/null | head -50
echo "##PROCESSES"
ps -eo pid,user,pcpu,comm --sort=-pcpu 2>/dev/null | awk 'NR>1' | head -10
`

// splitDiscoverySections 按 "##XXX" 行把输出切成 map。
func splitDiscoverySections(raw string) map[string]string {
	out := map[string]string{}
	current := ""
	var buf strings.Builder
	flush := func() {
		if current != "" {
			out[current] = strings.TrimSpace(buf.String())
		}
		buf.Reset()
	}
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "##") {
			flush()
			current = strings.TrimPrefix(trimmed, "##")
			continue
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	flush()
	return out
}

// parseHostname 把 HOSTNAME 段写到 server 节点 attrs。
func parseHostname(g *TopologyGraph, serverID, raw string) {
	host := strings.TrimSpace(raw)
	if host == "" {
		return
	}
	for i := range g.Nodes {
		if g.Nodes[i].ID == serverID {
			if g.Nodes[i].Attrs == nil {
				g.Nodes[i].Attrs = map[string]string{}
			}
			g.Nodes[i].Attrs["hostname"] = host
			return
		}
	}
}

// ss 的进程列形如 `users:(("nginx",pid=1234,fd=6))`，可能多个 user 并列。
// 我们只取第一个进程名，已经够用。
var ssProcessRE = regexp.MustCompile(`users:\(\("([^"]+)"`)

// netstat 的进程列形如 `1234/nginx`（数字 PID + 斜杠 + 程序名）。
var netstatProcessRE = regexp.MustCompile(`\b\d+/([A-Za-z0-9._\-]+)`)

// addrPortRE 匹配 "1.2.3.4:80" / "[::]:443" / "*:5432" 等地址:端口形态，仅捕获端口数字。
// 端口必须 1~65535，且要求 ":" 前有内容（避免误匹配纯端口号）。
var addrPortRE = regexp.MustCompile(`(?:\d+\.\d+\.\d+\.\d+|\[[0-9a-fA-F:]+\]|\*):(\d{1,5})\b`)

// parseListens 解析 ss/netstat 输出，提取监听端口节点。
// 行级解析按字段切分：proto 是第一列，端口走 addrPortRE 抓首个候选，
// 进程名按 ss/netstat 两种语法兜底匹配。
func parseListens(g *TopologyGraph, serverID, cfgID, raw string) {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		// 跳过表头：ss 的 "State Recv-Q ..."、netstat 的 "Proto Recv-Q ..."。
		if strings.HasPrefix(lower, "netid") || strings.HasPrefix(lower, "proto") ||
			strings.HasPrefix(lower, "state") || strings.HasPrefix(lower, "active internet") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		proto := strings.ToLower(fields[0])
		if !strings.HasPrefix(proto, "tcp") && !strings.HasPrefix(proto, "udp") {
			continue
		}
		// 找第一个本地 "addr:port"；ss 的 Local Address 是中间列，netstat 类似。
		// 跳过端口为 0 / "*" 的尾段。
		var port string
		for _, m := range addrPortRE.FindAllStringSubmatch(line, -1) {
			candidate := m[1]
			if candidate == "0" {
				continue
			}
			port = candidate
			break
		}
		if port == "" {
			continue
		}
		// 进程名两种来源；先 ss，回退 netstat。
		process := ""
		if m := ssProcessRE.FindStringSubmatch(line); m != nil {
			process = m[1]
		} else if m := netstatProcessRE.FindStringSubmatch(line); m != nil {
			process = m[1]
		}
		label := port + "/" + proto
		if process != "" {
			label = process + ":" + port
		}
		portID := NewNodeID(NodeKindPort, cfgID, port, proto)
		g.AddNode(TopologyNode{
			ID: portID, Kind: NodeKindPort, Label: label,
			Attrs: map[string]string{"port": port, "proto": proto, "process": process},
		})
		g.AddEdge(TopologyEdge{From: serverID, To: portID, Kind: EdgeContains})
		g.Evidence = append(g.Evidence, TopologyEvidence{
			SubjectID: portID, Kind: "ssh_listen",
			Snippet: truncate(line, 200),
		})
	}
}

// parseDocker 解析 docker ps 的 "name|image|ports" 输出。
func parseDocker(g *TopologyGraph, serverID, cfgID, raw string) {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		image := strings.TrimSpace(parts[1])
		if name == "" {
			continue
		}
		containerID := NewNodeID(NodeKindContainer, cfgID, name)
		attrs := map[string]string{"image": image}
		if len(parts) == 3 {
			attrs["ports"] = strings.TrimSpace(parts[2])
		}
		g.AddNode(TopologyNode{
			ID: containerID, Kind: NodeKindContainer,
			Label: name, Attrs: attrs,
		})
		g.AddEdge(TopologyEdge{From: serverID, To: containerID, Kind: EdgeContains})
		g.Evidence = append(g.Evidence, TopologyEvidence{
			SubjectID: containerID, Kind: "ssh_docker_ps",
			Snippet: truncate(line, 200),
		})
	}
}

// parseProcesses 取 CPU 占用前 N 进程，作为补充节点。
// 用 fields 拆分而不是 regex：ps 输出格式很稳定，列宽对齐用空白分隔。
func parseProcesses(g *TopologyGraph, serverID, cfgID, raw string) {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		pid, user, pcpu, comm := fields[0], fields[1], fields[2], fields[3]
		// 只把 CPU > 1% 的进程加入图，避免图被一堆 0% 的内核线程淹没
		if pcpu == "0.0" || pcpu == "0" {
			continue
		}
		procID := NewNodeID(NodeKindProcess, cfgID, pid)
		g.AddNode(TopologyNode{
			ID: procID, Kind: NodeKindProcess, Label: comm,
			Attrs: map[string]string{"pid": pid, "user": user, "pcpu": pcpu},
		})
		g.AddEdge(TopologyEdge{From: serverID, To: procID, Kind: EdgeRuns})
	}
}

// truncate 把长字符串截断到 maxBytes，附加省略号。
func truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes] + "..."
}

// displayName 在名称为空时回退到 ID。
func displayName(name, id string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return id
}
