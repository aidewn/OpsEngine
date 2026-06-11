// execsum 把 ExecutionRecord 压缩成 LLM 可消费的结构化文本。
// 独立成包的原因：runtime（修复 handler）与 tools/builtin（get_execution 工具）都需要它，
// 而 runtime 依赖 tools，不能反向引用。
package execsum

import (
	"fmt"
	"strings"

	"OpsEngine/internal/core"
)

const (
	// maxLogsPerNode 单个失败节点最多收集的日志条数（取尾部，错误信息一般在最后）。
	maxLogsPerNode = 20
	// maxLogLineLen 单条日志的最大字符数。
	maxLogLineLen = 300
	// maxVarValueLen 单个变量值的最大字符数（变量可能是整个文件内容）。
	maxVarValueLen = 500
	// maxTotalLen 整体输出上限，超出截断，避免撑爆模型上下文。
	maxTotalLen = 8000
)

// FailedNode 是一个失败节点的提取结果。
type FailedNode struct {
	InstanceID string
	TypeID     string
	Config     map[string]any
	Logs       []core.LogEntry
	// FramePath 描述节点所在调用栈位置，主流为 "main"，集合内为 "main > 集合名"。
	FramePath string
	// Variables 是失败时所在 frame 的变量快照。
	Variables map[string]any
}

// CollectFailedNodes 递归遍历 frame 树，收集所有 Failed 状态的节点及其上下文。
func CollectFailedNodes(rec core.ExecutionRecord) []FailedNode {
	var out []FailedNode
	collectFrame(rec, rec.RootFrame, "main", &out)
	return out
}

// collectFrame 处理单个 frame：定位失败节点并递归 children。
func collectFrame(rec core.ExecutionRecord, frame core.FrameState, path string, out *[]FailedNode) {
	nodes := frameNodes(rec, frame)
	for instanceID, state := range frame.NodeStates {
		if state != core.NodeStateFailed {
			continue
		}
		fn := FailedNode{
			InstanceID: instanceID,
			FramePath:  path,
			Logs:       tailLogs(frame.NodeLogs[instanceID]),
			Variables:  frame.Variables,
		}
		if n, ok := nodes[instanceID]; ok {
			fn.TypeID = n.TypeID
			fn.Config = n.Config
		}
		*out = append(*out, fn)
	}
	for callerID, child := range frame.Children {
		if child == nil {
			continue
		}
		childPath := path + " > " + frameLabel(rec, *child, callerID)
		collectFrame(rec, *child, childPath, out)
	}
}

// frameNodes 返回 frame 对应的节点定义表：主流取快照工作流，集合帧取快照集合。
func frameNodes(rec core.ExecutionRecord, frame core.FrameState) map[string]core.NodeInstance {
	var nodes []core.NodeInstance
	if frame.AssembleID == "" {
		nodes = rec.Snapshot.Workflow.Nodes
	} else if asm, ok := rec.Snapshot.Assembles[frame.AssembleID]; ok {
		nodes = asm.Nodes
	}
	out := make(map[string]core.NodeInstance, len(nodes))
	for _, n := range nodes {
		out[n.InstanceID] = n
	}
	return out
}

// frameLabel 取集合帧的可读名（找不到时退回 caller 实例 ID）。
func frameLabel(rec core.ExecutionRecord, frame core.FrameState, callerID string) string {
	if asm, ok := rec.Snapshot.Assembles[frame.AssembleID]; ok && asm.Name != "" {
		return asm.Name
	}
	return callerID
}

// tailLogs 取日志尾部 maxLogsPerNode 条，并截断超长行。
func tailLogs(logs []core.LogEntry) []core.LogEntry {
	if len(logs) > maxLogsPerNode {
		logs = logs[len(logs)-maxLogsPerNode:]
	}
	out := make([]core.LogEntry, len(logs))
	for i, l := range logs {
		l.Message = truncate(l.Message, maxLogLineLen)
		out[i] = l
	}
	return out
}

// FailureContext 生成执行失败的结构化文本，供「AI 修复」作为提示词上下文。
// 输出包含：执行概要、每个失败节点的类型/配置/日志尾部/frame 变量。
func FailureContext(rec core.ExecutionRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "执行 ID: %s\n工作流: %s（ID %s）\n状态: %s\n",
		rec.ID, rec.Snapshot.Workflow.Name, rec.WorkflowID, rec.Status)
	if rec.Error != "" {
		fmt.Fprintf(&b, "执行级错误: %s\n", truncate(rec.Error, maxLogLineLen))
	}

	failed := CollectFailedNodes(rec)
	if len(failed) == 0 {
		b.WriteString("未定位到 Failed 状态的节点（可能在启动阶段或调度层失败）。\n")
		return b.String()
	}
	fmt.Fprintf(&b, "失败节点 %d 个：\n", len(failed))
	for i, fn := range failed {
		fmt.Fprintf(&b, "\n## 失败节点 %d：%s（type=%s，位置 %s）\n", i+1, fn.InstanceID, fn.TypeID, fn.FramePath)
		if len(fn.Config) > 0 {
			b.WriteString("配置:\n")
			for k, v := range fn.Config {
				fmt.Fprintf(&b, "  %s = %s\n", k, truncate(fmt.Sprintf("%v", v), maxVarValueLen))
			}
		}
		if len(fn.Logs) > 0 {
			fmt.Fprintf(&b, "日志（尾部 %d 条）:\n", len(fn.Logs))
			for _, l := range fn.Logs {
				fmt.Fprintf(&b, "  [%s] %s\n", l.Level, l.Message)
			}
		}
		if len(fn.Variables) > 0 {
			b.WriteString("失败时变量:\n")
			for k, v := range fn.Variables {
				fmt.Fprintf(&b, "  %s = %s\n", k, truncate(fmt.Sprintf("%v", v), maxVarValueLen))
			}
		}
	}
	return truncate(b.String(), maxTotalLen)
}

// Summary 生成执行记录的简短摘要，供 get_execution 工具返回给模型。
// 成功执行只给概要；失败执行附带 FailureContext。
func Summary(rec core.ExecutionRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "执行 %s · 工作流「%s」· 状态 %s · 开始 %s",
		rec.ID, rec.Snapshot.Workflow.Name, rec.Status, rec.StartedAt.Format("2006-01-02 15:04:05"))
	if rec.FinishedAt != nil {
		fmt.Fprintf(&b, " · 结束 %s", rec.FinishedAt.Format("15:04:05"))
	}
	b.WriteByte('\n')
	switch rec.Status {
	case core.WorkflowStatusFailed, core.WorkflowStatusTerminated:
		b.WriteString(FailureContext(rec))
	default:
		fmt.Fprintf(&b, "节点 %d 个，无失败节点。\n", len(rec.Snapshot.Workflow.Nodes))
	}
	return b.String()
}

// truncate 把文本截断到 limit 字符（按 rune 计），超出附加省略标记。
func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…(已截断)"
}
