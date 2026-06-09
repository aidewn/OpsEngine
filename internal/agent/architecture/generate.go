// 架构报告（OpsDoc Kind=architecture）的高层编排：
//   - 把拓扑摘要喂给 LLM 拿"架构说明"段
//   - 拼装 Markdown：概览 → 主机摘要 → 架构说明 → Mermaid 图 → 采集证据 → 错误备注
//
// Summarizer 可为 nil：此时跳过"架构说明"段，仅产事实层报告。

package architecture

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"OpsEngine/internal/agent/prompt"
	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"

	"github.com/google/uuid"
)

// Summarizer 与 report 包同形态：接收一组 chat 消息，返回模型回复。
// 由 ai.go 用同一个 llmAdapter 注入。
type Summarizer func(messages []clients.ChatMessage) (string, error)

// GenerateArchitectureDoc 产出一份 OpsDoc。
func GenerateArchitectureDoc(env core.EnvironmentDef, g TopologyGraph, summarize Summarizer) (core.OpsDoc, error) {
	summary := RenderTopologySummary(g)

	explanation := ""
	if summarize != nil {
		text, err := summarizeArchitecture(summary, summarize)
		if err != nil {
			explanation = fmt.Sprintf("> LLM 架构说明失败：%s\n>\n> 报告事实部分仍然可用，可稍后重新生成。", err.Error())
		} else {
			explanation = strings.TrimSpace(text)
		}
	}

	body := renderArchitectureDoc(env, g, summary, explanation)
	now := time.Now()
	return core.OpsDoc{
		ID:    uuid.New().String(),
		Kind:  core.OpsDocKindArchitecture,
		Title: fmt.Sprintf("架构分析：%s", displayName(env.Name, env.ID)),
		Source: core.OpsDocSource{
			EnvironmentID: env.ID,
		},
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// summarizeArchitecture 让 LLM 基于拓扑事实输出架构说明。
func summarizeArchitecture(summary string, summarize Summarizer) (string, error) {
	systemPrompt, err := prompt.BuildArchitectureExplanationPrompt(summary)
	if err != nil {
		return "", err
	}
	return summarize([]clients.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "请基于上面的拓扑事实生成架构说明段。"},
	})
}

// RenderTopologySummary 把拓扑压成给 LLM 看的紧凑事实文本（不含 Mermaid）。
// 行行紧凑、每条事实带节点 id，方便 LLM 引用。
func RenderTopologySummary(g TopologyGraph) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "环境: %s (id=%s)\n", displayName(g.EnvironmentName, g.EnvironmentID), g.EnvironmentID)
	fmt.Fprintf(&sb, "节点统计: 主机=%d, 端口=%d, 容器=%d, 进程=%d\n",
		g.CountByKind(NodeKindServer), g.CountByKind(NodeKindPort),
		g.CountByKind(NodeKindContainer), g.CountByKind(NodeKindProcess))

	// 按主机分组列举附属节点，提升 LLM 理解效率。
	servers := nodesByKind(g.Nodes, NodeKindServer)
	for _, srv := range servers {
		fmt.Fprintf(&sb, "\n主机 %s (id=%s)", srv.Label, srv.ID)
		if host := srv.Attrs["hostname"]; host != "" {
			fmt.Fprintf(&sb, " hostname=%s", host)
		}
		sb.WriteString("\n")
		// 找它的子节点（通过 edges 反查）
		ports, containers, processes := childNodesOf(g, srv.ID)
		if len(ports) > 0 {
			sb.WriteString("  端口:\n")
			for _, p := range ports {
				fmt.Fprintf(&sb, "    - %s (id=%s)\n", p.Label, p.ID)
			}
		}
		if len(containers) > 0 {
			sb.WriteString("  容器:\n")
			for _, c := range containers {
				img := c.Attrs["image"]
				fmt.Fprintf(&sb, "    - %s image=%s (id=%s)\n", c.Label, img, c.ID)
			}
		}
		if len(processes) > 0 {
			sb.WriteString("  CPU 高的进程:\n")
			for _, p := range processes {
				fmt.Fprintf(&sb, "    - %s pcpu=%s%% (id=%s)\n", p.Label, p.Attrs["pcpu"], p.ID)
			}
		}
	}

	if len(g.CollectionErrors) > 0 {
		sb.WriteString("\n采集失败的主机:\n")
		for _, e := range g.CollectionErrors {
			fmt.Fprintf(&sb, "  - %s\n", e)
		}
	}
	return sb.String()
}

// renderArchitectureDoc 拼装最终 Markdown：概览 + 详情 + LLM 说明 + Mermaid + 备注。
func renderArchitectureDoc(env core.EnvironmentDef, g TopologyGraph, summary, explanation string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# 架构分析：%s\n\n", displayName(env.Name, env.ID))

	sb.WriteString("## 概览\n\n")
	fmt.Fprintf(&sb, "- 环境：%s（`%s`）\n", displayName(env.Name, env.ID), env.ID)
	fmt.Fprintf(&sb, "- 节点统计：主机 %d / 端口 %d / 容器 %d / CPU 高进程 %d\n",
		g.CountByKind(NodeKindServer), g.CountByKind(NodeKindPort),
		g.CountByKind(NodeKindContainer), g.CountByKind(NodeKindProcess))
	fmt.Fprintf(&sb, "- 生成时间：%s\n\n", time.Now().Format(time.RFC3339))

	if len(g.CollectionErrors) > 0 {
		sb.WriteString("> ⚠️ 以下主机采集失败，对应数据缺失：\n>\n")
		for _, e := range g.CollectionErrors {
			fmt.Fprintf(&sb, "> - %s\n", e)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## 拓扑事实\n\n")
	sb.WriteString("```\n")
	sb.WriteString(summary)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## 架构说明\n\n")
	if explanation != "" {
		sb.WriteString(explanation)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("（未启用 LLM 架构说明；可在 AI 设置中配置 API Key 后重新生成。）\n\n")
	}

	sb.WriteString("## 架构图\n\n")
	sb.WriteString(RenderMermaid(g))
	sb.WriteString("\n\n")
	sb.WriteString("> 当前前端以源码形式展示 Mermaid，可复制到 mermaid.live 查看渲染效果。\n")
	return sb.String()
}

// nodesByKind 取所有指定 kind 的节点，按 Label 字典序输出。
func nodesByKind(nodes []TopologyNode, kind NodeKind) []TopologyNode {
	out := []TopologyNode{}
	for _, n := range nodes {
		if n.Kind == kind {
			out = append(out, n)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

// childNodesOf 根据 edges 找某主机直接挂的端口/容器/进程。
func childNodesOf(g TopologyGraph, serverID string) (ports, containers, processes []TopologyNode) {
	byID := map[string]TopologyNode{}
	for _, n := range g.Nodes {
		byID[n.ID] = n
	}
	for _, e := range g.Edges {
		if e.From != serverID {
			continue
		}
		child, ok := byID[e.To]
		if !ok {
			continue
		}
		switch child.Kind {
		case NodeKindPort:
			ports = append(ports, child)
		case NodeKindContainer:
			containers = append(containers, child)
		case NodeKindProcess:
			processes = append(processes, child)
		}
	}
	sort.SliceStable(ports, func(i, j int) bool { return ports[i].Label < ports[j].Label })
	sort.SliceStable(containers, func(i, j int) bool { return containers[i].Label < containers[j].Label })
	sort.SliceStable(processes, func(i, j int) bool { return processes[i].Label < processes[j].Label })
	return
}
