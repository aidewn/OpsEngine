// 采集计划：把启用监控项的数据需求汇总、去重成最小采集任务集合。

package monitor

import (
	"encoding/json"

	"OpsEngine/internal/core"
)

// CollectionTask 是去重后的单次采集任务。
type CollectionTask struct {
	SourceID string
	TargetID string
	Kind     string
	Params   map[string]any
	key      string // 规范化去重键，包内部使用
}

// Key 返回采集任务的规范化去重键（source|target|kind|canonical(params)）。
func (t CollectionTask) Key() string { return t.key }

// ExtractRequirements 汇总所有启用监控项的数据需求。
// 跳过未启用的监控项；不去重（去重交给 BuildCollectionPlan）。
func ExtractRequirements(panels []core.MonitorPanel) []core.DataRequirement {
	reqs := []core.DataRequirement{}
	for _, p := range panels {
		if !p.Enabled {
			continue
		}
		reqs = append(reqs, p.Requirements...)
	}
	return reqs
}

// BuildCollectionPlan 按 监控源+目标+类型+参数 去重，得到本轮要执行的采集任务。
// 多个监控项引用同一目标同一数据时只采集一次（plan §4.1）。
func BuildCollectionPlan(reqs []core.DataRequirement) []CollectionTask {
	seen := map[string]bool{}
	plan := []CollectionTask{}
	for _, r := range reqs {
		sourceID := normalizeSourceID(r.SourceID)
		key := collectionKey(sourceID, r.TargetID, r.Kind, r.Params)
		if seen[key] {
			continue
		}
		seen[key] = true
		plan = append(plan, CollectionTask{
			SourceID: sourceID,
			TargetID: r.TargetID,
			Kind:     r.Kind,
			Params:   r.Params,
			key:      key,
		})
	}
	return plan
}

// collectionKey 生成规范化去重键。
// encoding/json 对 map[string]any 的键按字母序输出，结果稳定，可直接作去重键；
// 空参数（nil 或空 map）统一视为同一键，避免 nil 与 {} 被当成两次采集。
func collectionKey(sourceID, targetID, kind string, params map[string]any) string {
	raw := ""
	if len(params) > 0 {
		b, _ := json.Marshal(params)
		raw = string(b)
	}
	return normalizeSourceID(sourceID) + "|" + targetID + "|" + kind + "|" + raw
}

// normalizeSourceID 为空时回落到 builtin，兼容旧监控项。
func normalizeSourceID(sourceID string) string {
	if sourceID == "" {
		return core.MonitorBuiltinSourceID
	}
	return sourceID
}
