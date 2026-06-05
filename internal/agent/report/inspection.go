// 巡检报告生成。
//
// 流水线（文档 P5 / P10）：
//
//   ExecutionRecord ──▶ ExtractFacts ──▶ InspectionReport (纯事实)
//                                           │
//                                           ├──▶ Render (Markdown)
//                                           │
//                                           └──▶ optional LLM Risk Analysis (追加风险/建议)
//
// 设计要点：
//   - 事实提取完全确定性，无 LLM，便于单测
//   - LLM 风险分析是可选层，未配 API Key 时退化为纯事实报告
//   - 报告以 core.OpsDoc 形态持久化（由上层 ai.go 完成 Store.Save）

package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"OpsEngine/internal/core"
)

// inspectionItemNodeType 是巡检工作流中承载每项检查的节点类型。
// 与 internal/agent/inspection/materialize.go 中固定使用的 type_id 对齐。
const inspectionItemNodeType = "linux_exec_command"

// InspectionReport 是结构化的巡检报告事实层（无 LLM 干预）。
// Render 时会把它转成 Markdown 注入到 OpsDoc.Body。
type InspectionReport struct {
	EnvironmentID   string
	EnvironmentName string
	WorkflowID      string
	WorkflowName    string
	ExecutionID     string
	OverallStatus   core.WorkflowStatus
	StartedAt       time.Time
	FinishedAt      *time.Time
	Items           []InspectionItemResult
}

// InspectionItemResult 是单项巡检在执行后的事实结果。
type InspectionItemResult struct {
	NodeID  string         // workflow 中的节点实例 ID
	Title   string         // 来自 Plan，记录在 node.Config["title"]
	Command string         // 来自 Plan，记录在 node.Config["command"]
	State   core.NodeState // Success / Failed / Skipped / ...
	Logs    []string       // 节点日志（按时间顺序，已截断长输出）
}

// Counts 汇总各状态的项数，模板渲染用。
func (r InspectionReport) Counts() map[core.NodeState]int {
	out := map[core.NodeState]int{}
	for _, it := range r.Items {
		out[it.State]++
	}
	return out
}

// HasFailures 报告是否包含失败项（含 Terminated）。Render 据此决定是否提示"需关注"。
func (r InspectionReport) HasFailures() bool {
	for _, it := range r.Items {
		if it.State == core.NodeStateFailed || it.State == core.NodeStateTerminated {
			return true
		}
	}
	return false
}

// ExtractFacts 从工作流定义 + 执行记录提取结构化巡检结果。
// 只处理主流（RootFrame）下的 linux_exec_command 节点，按 workflow.Nodes 顺序输出。
//
// 注意：当前实现假定巡检工作流的所有项都在主流上（inspection.Materialize 就是这样产出的）。
// 后续若有集合调用形态的巡检，需要扩展为遍历 Children frames。
func ExtractFacts(env core.EnvironmentDef, wf core.WorkflowDef, rec core.ExecutionRecord) InspectionReport {
	items := make([]InspectionItemResult, 0, len(wf.Nodes))
	for _, node := range wf.Nodes {
		if node.TypeID != inspectionItemNodeType {
			continue
		}
		title := configString(node.Config, "title")
		if strings.TrimSpace(title) == "" {
			// 没有 title 的 linux_exec_command 不视为巡检项（可能是手工拼的工作流）
			continue
		}
		state := rec.RootFrame.NodeStates[node.InstanceID]
		items = append(items, InspectionItemResult{
			NodeID:  node.InstanceID,
			Title:   title,
			Command: configString(node.Config, "command"),
			State:   state,
			Logs:    formatLogs(rec.RootFrame.NodeLogs[node.InstanceID]),
		})
	}
	return InspectionReport{
		EnvironmentID:   env.ID,
		EnvironmentName: env.Name,
		WorkflowID:      wf.ID,
		WorkflowName:    wf.Name,
		ExecutionID:     rec.ID,
		OverallStatus:   rec.Status,
		StartedAt:       rec.StartedAt,
		FinishedAt:      rec.FinishedAt,
		Items:           items,
	}
}

// Render 把 InspectionReport 渲染为 Markdown。
// riskSection 是可选的"风险与建议"段（由 LLM 产出）；空字符串时跳过该段。
func Render(r InspectionReport, riskSection string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# 巡检报告：%s\n\n", r.WorkflowName)
	sb.WriteString("## 巡检概览\n\n")
	fmt.Fprintf(&sb, "- 环境：%s\n", displayName(r.EnvironmentName, r.EnvironmentID))
	fmt.Fprintf(&sb, "- 工作流：%s\n", displayName(r.WorkflowName, r.WorkflowID))
	fmt.Fprintf(&sb, "- 执行 ID：`%s`\n", r.ExecutionID)
	fmt.Fprintf(&sb, "- 开始时间：%s\n", r.StartedAt.Format(time.RFC3339))
	if r.FinishedAt != nil {
		fmt.Fprintf(&sb, "- 结束时间：%s（耗时 %s）\n",
			r.FinishedAt.Format(time.RFC3339),
			r.FinishedAt.Sub(r.StartedAt).Round(time.Second))
	}
	fmt.Fprintf(&sb, "- 总体状态：**%s**\n", r.OverallStatus)
	sb.WriteString(renderCountsLine(r.Counts()))
	sb.WriteString("\n")

	if r.HasFailures() {
		sb.WriteString("> ⚠️ 报告中包含失败项，请关注下文「检查结果」中标记为 Failed 的条目。\n\n")
	}

	sb.WriteString("## 检查结果\n\n")
	if len(r.Items) == 0 {
		sb.WriteString("（未识别到任何巡检项；此工作流可能不是由巡检流程生成。）\n\n")
	}
	for i, it := range r.Items {
		fmt.Fprintf(&sb, "### %d. %s — %s\n\n", i+1, it.Title, stateBadge(it.State))
		if it.Command != "" {
			fmt.Fprintf(&sb, "```sh\n%s\n```\n\n", it.Command)
		}
		if len(it.Logs) == 0 {
			sb.WriteString("（无输出日志）\n\n")
			continue
		}
		sb.WriteString("```\n")
		for _, line := range it.Logs {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("```\n\n")
	}

	if strings.TrimSpace(riskSection) != "" {
		sb.WriteString("## 风险与建议\n\n")
		sb.WriteString(strings.TrimRight(riskSection, "\n"))
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("## 风险与建议\n\n")
		sb.WriteString("（未启用 LLM 风险分析；可在 AI 设置中配置 API Key 后重新生成。）\n\n")
	}

	return sb.String()
}

// renderCountsLine 输出形如 "- 项数统计：成功 5 / 失败 1 / 跳过 0"。
func renderCountsLine(counts map[core.NodeState]int) string {
	if len(counts) == 0 {
		return ""
	}
	// 固定排序：按状态字符串字典序，避免每次输出乱序。
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", stateLabel(core.NodeState(k)), counts[core.NodeState(k)]))
	}
	return "- 项数统计：" + strings.Join(parts, " / ") + "\n"
}

// stateBadge 状态显示标记，给 Markdown 标题用。
func stateBadge(s core.NodeState) string {
	switch s {
	case core.NodeStateSuccess:
		return "✅ 成功"
	case core.NodeStateFailed:
		return "❌ 失败"
	case core.NodeStateSkipped:
		return "⏭ 跳过"
	case core.NodeStateTerminated:
		return "⛔ 中断"
	case core.NodeStateExecuting:
		return "⏳ 执行中"
	case core.NodeStateIdle, "":
		return "⚪ 未执行"
	default:
		return string(s)
	}
}

// stateLabel 短中文，给统计行用。
func stateLabel(s core.NodeState) string {
	switch s {
	case core.NodeStateSuccess:
		return "成功"
	case core.NodeStateFailed:
		return "失败"
	case core.NodeStateSkipped:
		return "跳过"
	case core.NodeStateTerminated:
		return "中断"
	case core.NodeStateExecuting:
		return "执行中"
	case core.NodeStateIdle, "":
		return "未执行"
	default:
		return string(s)
	}
}

// formatLogs 把节点日志转为可读字符串行，并对单条过长输出做截断。
// 单条日志最长 1000 字节，整体最多 50 条；防止日志膨胀让报告失控。
func formatLogs(entries []core.LogEntry) []string {
	const maxLines = 50
	const maxLineBytes = 1000
	if len(entries) == 0 {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if len(out) >= maxLines {
			out = append(out, "... (剩余日志已省略)")
			break
		}
		msg := e.Message
		if len(msg) > maxLineBytes {
			msg = msg[:maxLineBytes] + "...(截断)"
		}
		out = append(out, fmt.Sprintf("[%s] %s", e.Level, msg))
	}
	return out
}

// configString 安全地从节点 Config 取字符串字段。
func configString(cfg map[string]any, key string) string {
	if cfg == nil {
		return ""
	}
	v, _ := cfg[key].(string)
	return v
}

// displayName 在名称为空时回退到 ID，避免出现"环境： / xxx"这种空段。
func displayName(name, id string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return id
}
