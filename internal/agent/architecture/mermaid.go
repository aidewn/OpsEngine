// 把 TopologyGraph 渲染成 Mermaid 源码。
// 选 `graph TD`（top-down 流向图），层次清晰：环境 → 主机 → (容器 / 端口 / 进程)。
//
// Mermaid 节点 id 字符集限制比较严，包含冒号/斜杠会渲染失败。
// 我们的 NodeID 含 ":" 与 "/"——这里统一把它们映射到内部短 id（n0/n1/...），
// 同时把原始 ID 写到 label 注释里方便审计。

package architecture

import (
	"fmt"
	"sort"
	"strings"
)

// RenderMermaid 把 TopologyGraph 渲染成 graph TD 形式的 Mermaid 源。
// 输出整体被 ```mermaid ... ``` 围栏包裹，方便直接插入 Markdown。
func RenderMermaid(g TopologyGraph) string {
	if len(g.Nodes) == 0 {
		return "```mermaid\ngraph TD\n  empty[(空环境)]\n```"
	}
	mapper := newIDMapper()
	var sb strings.Builder
	sb.WriteString("```mermaid\ngraph TD\n")

	// 节点：按 Kind 分组排序，保证渲染顺序稳定且层次清楚。
	sorted := sortedNodes(g.Nodes)
	for _, n := range sorted {
		short := mapper.get(n.ID)
		shape := mermaidShape(n.Kind, escapeLabel(formatNodeLabel(n)))
		fmt.Fprintf(&sb, "  %s%s\n", short, shape)
	}

	// 边：稳定排序，保证 diff 友好。
	edges := make([]TopologyEdge, len(g.Edges))
	copy(edges, g.Edges)
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	for _, e := range edges {
		from, ok1 := mapper.lookup(e.From)
		to, ok2 := mapper.lookup(e.To)
		if !ok1 || !ok2 {
			continue
		}
		if strings.TrimSpace(e.Label) != "" {
			fmt.Fprintf(&sb, "  %s -- %s --> %s\n", from, escapeLabel(e.Label), to)
		} else {
			fmt.Fprintf(&sb, "  %s --> %s\n", from, to)
		}
	}

	// classDef 给不同节点上色，让人眼能一眼区分环境/主机/端口/容器/进程。
	sb.WriteString("\n  classDef env fill:#1e293b,color:#fff,stroke:#0f172a\n")
	sb.WriteString("  classDef server fill:#0ea5e9,color:#fff,stroke:#0369a1\n")
	sb.WriteString("  classDef port fill:#fef3c7,color:#92400e,stroke:#d97706\n")
	sb.WriteString("  classDef container fill:#c7d2fe,color:#3730a3,stroke:#4f46e5\n")
	sb.WriteString("  classDef process fill:#e5e7eb,color:#374151,stroke:#9ca3af\n")
	for _, n := range sorted {
		short, _ := mapper.lookup(n.ID)
		fmt.Fprintf(&sb, "  class %s %s\n", short, n.Kind)
	}
	sb.WriteString("```")
	return sb.String()
}

// sortedNodes 按 Kind 优先级再按 Label 排序，保证渲染顺序稳定。
// 顺序：env → server → container → port → process（自外向内）。
func sortedNodes(nodes []TopologyNode) []TopologyNode {
	priority := map[NodeKind]int{
		NodeKindEnv: 0, NodeKindServer: 1, NodeKindContainer: 2,
		NodeKindPort: 3, NodeKindProcess: 4,
	}
	out := make([]TopologyNode, len(nodes))
	copy(out, nodes)
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := priority[out[i].Kind], priority[out[j].Kind]
		if pi != pj {
			return pi < pj
		}
		return out[i].Label < out[j].Label
	})
	return out
}

// formatNodeLabel 把节点的 Label + 关键属性拼成一行 Mermaid 文本。
// 不同 Kind 暴露不同关键属性：
//   - server: hostname / user
//   - port:   port/proto (label 本身已经是 "nginx:80"/" :::5432")
//   - container: image
//   - process: pcpu
func formatNodeLabel(n TopologyNode) string {
	switch n.Kind {
	case NodeKindServer:
		if host := n.Attrs["hostname"]; host != "" {
			return fmt.Sprintf("%s\\n%s", n.Label, host)
		}
		return n.Label
	case NodeKindContainer:
		if img := n.Attrs["image"]; img != "" {
			return fmt.Sprintf("%s\\n%s", n.Label, img)
		}
		return n.Label
	case NodeKindProcess:
		if cpu := n.Attrs["pcpu"]; cpu != "" {
			return fmt.Sprintf("%s\\nCPU %s%%", n.Label, cpu)
		}
		return n.Label
	default:
		return n.Label
	}
}

// mermaidShape 根据 Kind 返回 Mermaid 节点形状语法。
//   - env 用粗体方框 [[ ]]（双层）
//   - server 用圆角方框 ( )（注意 Mermaid 圆角是 ([ ])）
//   - port 用六边形 {{ }}
//   - container 用菱形 { }（这里用 ([ ]) 圆角更直观）
//   - process 用方框 [ ]
func mermaidShape(kind NodeKind, label string) string {
	switch kind {
	case NodeKindEnv:
		return "[[\"" + label + "\"]]"
	case NodeKindServer:
		return "([\"" + label + "\"])"
	case NodeKindPort:
		return "{{\"" + label + "\"}}"
	case NodeKindContainer:
		return "[\"" + label + "\"]"
	case NodeKindProcess:
		return "[\"" + label + "\"]"
	default:
		return "[\"" + label + "\"]"
	}
}

// escapeLabel 把 Mermaid 节点/边标签中的特殊字符替换成安全形式。
// Mermaid 解析器对 "<>" / 反引号 / 双引号都比较敏感。
func escapeLabel(label string) string {
	label = strings.ReplaceAll(label, "\"", "'")
	label = strings.ReplaceAll(label, "`", "'")
	label = strings.ReplaceAll(label, "<", "&lt;")
	label = strings.ReplaceAll(label, ">", "&gt;")
	return label
}

// idMapper 把外部 NodeID（含 ":" "/"）映射到 Mermaid 安全的短 id (n0/n1/...)。
type idMapper struct {
	order []string
	index map[string]string
}

func newIDMapper() *idMapper { return &idMapper{index: map[string]string{}} }

func (m *idMapper) get(externalID string) string {
	if id, ok := m.index[externalID]; ok {
		return id
	}
	id := fmt.Sprintf("n%d", len(m.order))
	m.order = append(m.order, externalID)
	m.index[externalID] = id
	return id
}

func (m *idMapper) lookup(externalID string) (string, bool) {
	id, ok := m.index[externalID]
	return id, ok
}
