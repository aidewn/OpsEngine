// 工作流/集合的结构化 diff：按 instance_id 对齐节点，输出增/删/改与边变化。
// 用途：AI 更新资产后给用户一份可读的变更摘要（替代旧的"节点数 N → M"），
// 让"模型悄悄改了没让它动的东西"变得可见。
// 刻意不做语义合并——只描述差异，不解决冲突。

package workflow

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"OpsEngine/internal/core"
)

// NodeChange 描述单个节点的变更。
type NodeChange struct {
	InstanceID string   `json:"instance_id"`
	TypeID     string   `json:"type_id"`
	Fields     []string `json:"fields,omitempty"` // 仅 Changed 项使用：发生变更的 config 字段名
}

// GraphDiff 是一次图变更的汇总。
type GraphDiff struct {
	Added        []NodeChange `json:"added,omitempty"`
	Removed      []NodeChange `json:"removed,omitempty"`
	Changed      []NodeChange `json:"changed,omitempty"`
	EdgesAdded   int          `json:"edges_added,omitempty"`
	EdgesRemoved int          `json:"edges_removed,omitempty"`
}

// DiffWorkflows 对比两个工作流（按节点 instance_id 对齐）。
func DiffWorkflows(old, new core.WorkflowDef) GraphDiff {
	return DiffGraph(old.Nodes, new.Nodes, old.Edges, new.Edges)
}

// DiffGraph 对比节点与边集合，工作流与集合共用。
func DiffGraph(oldNodes, newNodes []core.NodeInstance, oldEdges, newEdges []core.EdgeConfig) GraphDiff {
	var d GraphDiff
	oldByID := make(map[string]core.NodeInstance, len(oldNodes))
	for _, n := range oldNodes {
		oldByID[n.InstanceID] = n
	}
	newByID := make(map[string]core.NodeInstance, len(newNodes))
	for _, n := range newNodes {
		newByID[n.InstanceID] = n
	}

	for _, n := range newNodes {
		prev, ok := oldByID[n.InstanceID]
		if !ok {
			d.Added = append(d.Added, NodeChange{InstanceID: n.InstanceID, TypeID: n.TypeID})
			continue
		}
		if fields := changedFields(prev, n); len(fields) > 0 {
			d.Changed = append(d.Changed, NodeChange{InstanceID: n.InstanceID, TypeID: n.TypeID, Fields: fields})
		}
	}
	for _, n := range oldNodes {
		if _, ok := newByID[n.InstanceID]; !ok {
			d.Removed = append(d.Removed, NodeChange{InstanceID: n.InstanceID, TypeID: n.TypeID})
		}
	}

	oldEdgeSet := edgeSet(oldEdges)
	newEdgeSet := edgeSet(newEdges)
	for k := range newEdgeSet {
		if !oldEdgeSet[k] {
			d.EdgesAdded++
		}
	}
	for k := range oldEdgeSet {
		if !newEdgeSet[k] {
			d.EdgesRemoved++
		}
	}
	return d
}

// changedFields 返回两个同 ID 节点之间发生变化的字段名（type 变化记为 "type_id"，位置变化忽略）。
func changedFields(old, new core.NodeInstance) []string {
	var fields []string
	if old.TypeID != new.TypeID {
		fields = append(fields, "type_id")
	}
	keys := map[string]bool{}
	for k := range old.Config {
		keys[k] = true
	}
	for k := range new.Config {
		keys[k] = true
	}
	for k := range keys {
		if !reflect.DeepEqual(old.Config[k], new.Config[k]) {
			fields = append(fields, k)
		}
	}
	sort.Strings(fields)
	return fields
}

// edgeSet 把边集合转为可比较的 key 集合。
func edgeSet(edges []core.EdgeConfig) map[string]bool {
	out := make(map[string]bool, len(edges))
	for _, e := range edges {
		out[fmt.Sprintf("%s:%s->%s:%s", e.From.Node, e.From.Port, e.To.Node, e.To.Port)] = true
	}
	return out
}

// IsEmpty 报告 diff 是否没有任何变更。
func (d GraphDiff) IsEmpty() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0 &&
		d.EdgesAdded == 0 && d.EdgesRemoved == 0
}

// Summary 生成单行可读摘要，用于会话消息与事件的 ChangeSummary 字段。
// 例：「新增 2 节点（linux_exec_command×2）；修改 print[ab12]（message）；边 +1/-1」
func (d GraphDiff) Summary() string {
	if d.IsEmpty() {
		return "无结构变更"
	}
	parts := []string{}
	if len(d.Added) > 0 {
		parts = append(parts, fmt.Sprintf("新增 %d 节点（%s）", len(d.Added), typeCounts(d.Added)))
	}
	if len(d.Removed) > 0 {
		parts = append(parts, fmt.Sprintf("删除 %d 节点（%s）", len(d.Removed), typeCounts(d.Removed)))
	}
	for _, c := range d.Changed {
		parts = append(parts, fmt.Sprintf("修改 %s[%s]（%s）", c.TypeID, shortID(c.InstanceID), strings.Join(c.Fields, ", ")))
	}
	if d.EdgesAdded > 0 || d.EdgesRemoved > 0 {
		parts = append(parts, fmt.Sprintf("边 +%d/-%d", d.EdgesAdded, d.EdgesRemoved))
	}
	return strings.Join(parts, "；")
}

// typeCounts 把节点列表按 type 聚合成 "type×N" 描述。
func typeCounts(nodes []NodeChange) string {
	count := map[string]int{}
	order := []string{}
	for _, n := range nodes {
		if count[n.TypeID] == 0 {
			order = append(order, n.TypeID)
		}
		count[n.TypeID]++
	}
	parts := make([]string, 0, len(order))
	for _, t := range order {
		if count[t] > 1 {
			parts = append(parts, fmt.Sprintf("%s×%d", t, count[t]))
		} else {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, ", ")
}

// shortID 取 instance_id 前 8 位用于摘要展示。
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
