// 环境拓扑的数据结构。
//
// 设计（文档 §3.1）：
//   - TopologyGraph 是事实层：从环境配置 + SSH 采集结果聚合而来，不含模型生成内容。
//   - Mermaid 是展示层产物，从 TopologyGraph 渲染，可重复重生成。
//   - 模型可以基于 TopologyGraph 生成"说明文字"，但不允许编造图中没有的节点和边。
//
// 节点 ID 规则（保证 Mermaid 节点 id 唯一且字符集安全）：
//   env:<envID>
//   server:<configID>
//   port:<configID>:<port>:<proto>
//   container:<configID>:<containerName>
//   process:<configID>:<pid>
// 调用方组装 ID 时统一走 NewNodeID，避免拼字符串出错。

package architecture

// NodeKind 是拓扑节点类型，决定 Mermaid 形状与渲染样式。
type NodeKind string

const (
	NodeKindEnv       NodeKind = "env"       // 顶层环境
	NodeKindServer    NodeKind = "server"    // SSH 主机
	NodeKindPort      NodeKind = "port"      // 监听端口（host:port）
	NodeKindContainer NodeKind = "container" // Docker 容器
	NodeKindProcess   NodeKind = "process"   // 进程（pid + name）
)

// EdgeKind 是拓扑边类型，描述两个节点之间的关系。
type EdgeKind string

const (
	EdgeContains EdgeKind = "contains" // 环境包含主机 / 主机承载端口/容器
	EdgeListens  EdgeKind = "listens"  // 端口被某进程/容器监听（未来扩展）
	EdgeRuns     EdgeKind = "runs"     // 主机运行某进程/容器
)

// TopologyNode 是拓扑节点。
type TopologyNode struct {
	ID    string            `json:"id"`
	Kind  NodeKind          `json:"kind"`
	Label string            `json:"label"`           // Mermaid 显示文本
	Attrs map[string]string `json:"attrs,omitempty"` // 自由字段：image / user / pcpu / pid 等
}

// TopologyEdge 是拓扑边。From / To 必须是图中已存在的节点 ID。
type TopologyEdge struct {
	From  string   `json:"from"`
	To    string   `json:"to"`
	Kind  EdgeKind `json:"kind"`
	Label string   `json:"label,omitempty"`
}

// TopologyEvidence 记录某条事实来源，用于报告"判断/建议"段的可追溯性。
type TopologyEvidence struct {
	// SubjectID 是事实关联到的节点 ID（如 server:cfg-1）。
	SubjectID string `json:"subject_id"`
	// Kind 是证据来源类别，例如 "ssh_listen" / "ssh_docker_ps"。
	Kind string `json:"kind"`
	// Snippet 是原始片段（已截断），用于审计。
	Snippet string `json:"snippet"`
}

// TopologyGraph 是事实层完整拓扑。Nodes/Edges 必须自洽（边端点都在 Nodes 中）。
type TopologyGraph struct {
	EnvironmentID   string             `json:"environment_id"`
	EnvironmentName string             `json:"environment_name"`
	Nodes           []TopologyNode     `json:"nodes"`
	Edges           []TopologyEdge     `json:"edges"`
	Evidence        []TopologyEvidence `json:"evidence,omitempty"`
	// CollectionErrors 记录单台 SSH 采集失败的非致命错误，不影响图整体可用性。
	CollectionErrors []string `json:"collection_errors,omitempty"`
}

// NewNodeID 按 kind 与组成段拼出唯一 ID。
// 使用 ":" 作为分隔，调用方应保证段内不含 ":"（id 字段在我们的来源数据里都是 UUID/数字）。
func NewNodeID(kind NodeKind, parts ...string) string {
	id := string(kind)
	for _, p := range parts {
		id += ":" + p
	}
	return id
}

// AddNode 追加节点；若 ID 已存在则保留首个版本（防止重复定义打乱 Mermaid）。
// 不去重 ID 会让 Mermaid 出现两个同名节点，渲染时位置混乱。
func (g *TopologyGraph) AddNode(n TopologyNode) {
	for _, existing := range g.Nodes {
		if existing.ID == n.ID {
			return
		}
	}
	g.Nodes = append(g.Nodes, n)
}

// AddEdge 追加边；调用方负责保证 from/to 已经 AddNode。
// 完全相同的边（同 from/to/kind）去重，避免 Mermaid 重复连线。
func (g *TopologyGraph) AddEdge(e TopologyEdge) {
	for _, existing := range g.Edges {
		if existing.From == e.From && existing.To == e.To && existing.Kind == e.Kind {
			return
		}
	}
	g.Edges = append(g.Edges, e)
}

// CountByKind 统计某种 Kind 的节点数量，供概览段使用。
func (g *TopologyGraph) CountByKind(kind NodeKind) int {
	n := 0
	for _, node := range g.Nodes {
		if node.Kind == kind {
			n++
		}
	}
	return n
}
