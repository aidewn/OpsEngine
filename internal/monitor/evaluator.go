// Monitor Flow 与状态机（Phase 3）。
//
// Monitor Flow 是高频轻量判断（plan §6.1）：只看本轮采集结果、不连服务器、不调 AI，
// 对监控项的每条阈值条件求值，输出结构化状态。状态机据此在四态间迁移并更新 PanelState。

package monitor

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"OpsEngine/internal/core"
)

// FlowResult 是 Monitor Flow 的判断结果。
type FlowResult struct {
	Status             core.MonitorStatus
	Severity           core.MonitorSeverity
	Summary            string
	Evidence           []string
	ShouldTroubleshoot bool
}

// Evaluate 对监控项跑一轮 Monitor Flow：逐条件求值，任一命中即异常。
// batch 应为该监控项的数据切片（Batch.Slice 的结果）。
// 无条件时视为正常（用户尚未定义判断规则）。
func Evaluate(panel core.MonitorPanel, batch Batch) FlowResult {
	if len(panel.Conditions) == 0 {
		return FlowResult{Status: core.MonitorStatusNormal, Severity: core.MonitorSeverityNone, Summary: "未定义判断条件"}
	}

	var evidence []string
	severity := core.MonitorSeverityNone
	for _, cond := range panel.Conditions {
		res, ok := findResult(batch, cond.SourceID, cond.Kind, cond.TargetID)
		if !ok {
			// 缺少该数据（无对应需求或目标）：无法判断本身就是问题，判为异常并说明。
			evidence = append(evidence, fmt.Sprintf("%s 无采集数据，无法判断", cond.Kind))
			severity = maxSeverity(severity, normalizeSeverity(cond.Severity))
			continue
		}
		if res.Err != nil {
			// 采集失败（如目标不可达）：监控未能完成，判为异常，把错误作为证据。
			evidence = append(evidence, fmt.Sprintf("%s 采集失败: %v", cond.Kind, res.Err))
			severity = maxSeverity(severity, normalizeSeverity(cond.Severity))
			continue
		}
		values := extractNumbers(toMap(res.Data), strings.Split(cond.Field, "."))
		for _, v := range values {
			if compare(v, cond.Op, cond.Value) {
				evidence = append(evidence, fmt.Sprintf("%s.%s = %s %s %s",
					cond.Kind, cond.Field, trimFloat(v), cond.Op, trimFloat(cond.Value)))
				severity = maxSeverity(severity, normalizeSeverity(cond.Severity))
				break // 同一条件命中一次即可
			}
		}
	}

	if severity != core.MonitorSeverityNone {
		return FlowResult{
			Status:             core.MonitorStatusAbnormal,
			Severity:           severity,
			Summary:            strings.Join(evidence, "；"),
			Evidence:           evidence,
			ShouldTroubleshoot: true,
		}
	}
	return FlowResult{Status: core.MonitorStatusNormal, Severity: core.MonitorSeverityNone, Summary: "全部条件正常"}
}

// Thresholds 防抖阈值：连续异常/正常达到对应次数才翻转状态（plan §5.3）。
// 值 <=0 视为 1（即时翻转，不防抖）。
type Thresholds struct {
	Abnormal int
	Recovery int
}

func (t Thresholds) abnormal() int {
	if t.Abnormal <= 0 {
		return 1
	}
	return t.Abnormal
}

func (t Thresholds) recovery() int {
	if t.Recovery <= 0 {
		return 1
	}
	return t.Recovery
}

// NextPanelState 依据上一状态、本轮判断结果与防抖阈值计算新的 PanelState（plan §7 状态机）。
// 防抖：异常需连续 Abnormal 次才翻为 abnormal；恢复需连续 Recovery 次才解除。
// 未达阈值的「抖动」期间维持原状态，只累计计数，从而抑制报警风暴。
// Diagnosing 由手动诊断流程设置，不在此处产生。
func NextPanelState(prev core.PanelState, panelID string, result FlowResult, th Thresholds, now time.Time) core.PanelState {
	next := prev
	next.PanelID = panelID
	next.LastCheckedAt = &now

	// 诊断进行中：冻结状态，避免后台 tick 把「诊断中」覆盖掉；待诊断流程结束再恢复。
	if prev.Status == core.MonitorStatusDiagnosing {
		return next
	}

	if result.Status == core.MonitorStatusAbnormal {
		next.ConsecutiveAbnormal = prev.ConsecutiveAbnormal + 1
		next.ConsecutiveNormal = 0
		// 已是异常态则维持；否则需连续达阈值才翻转。
		if prev.Status == core.MonitorStatusAbnormal || next.ConsecutiveAbnormal >= th.abnormal() {
			next.Status = core.MonitorStatusAbnormal
			next.Severity = result.Severity
			next.Summary = result.Summary
			next.HasHistory = true
		}
		// 未达阈值：保持 prev 状态（仍在累计），不告警。
		return next
	}

	// 本轮正常。
	next.ConsecutiveNormal = prev.ConsecutiveNormal + 1
	next.ConsecutiveAbnormal = 0
	wasAbnormal := prev.Status == core.MonitorStatusAbnormal || prev.Status == core.MonitorStatusDiagnosing
	if wasAbnormal {
		// 恢复需连续达阈值；未达则维持异常态，避免抖动误恢复。
		if next.ConsecutiveNormal < th.recovery() {
			return next
		}
		next.Status = core.MonitorStatusHistory
		next.Severity = core.MonitorSeverityNone
		next.HasHistory = true
		next.Summary = "已恢复，保留最近异常历史"
		return next
	}

	// 原本就正常：有异常历史显示「正常（有异常历史）」，否则纯正常。
	next.Severity = core.MonitorSeverityNone
	if prev.HasHistory {
		next.Status = core.MonitorStatusHistory
		next.Summary = "已恢复，保留最近异常历史"
	} else {
		next.Status = core.MonitorStatusNormal
		next.Summary = result.Summary
	}
	return next
}

// ── 取值与比较 ──────────────────────────────────────────

// findResult 在切片后的 batch 中查匹配 source、kind（可选 target）的结果。
func findResult(batch Batch, sourceID, kind, targetID string) (CollectionResult, bool) {
	sourceID = normalizeSourceID(sourceID)
	for _, res := range batch {
		if normalizeSourceID(res.Task.SourceID) != sourceID {
			continue
		}
		if res.Task.Kind != kind {
			continue
		}
		if targetID != "" && res.Task.TargetID != targetID {
			continue
		}
		return res, true
	}
	return CollectionResult{}, false
}

// toMap 把任意结构（带 json tag）转成 map[string]any，便于按字段路径取值。
func toMap(data any) any {
	if data == nil {
		return nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

// extractNumbers 按路径从 node 取数值，支持单层数组展开（segment 以 "[]" 结尾）。
func extractNumbers(node any, segs []string) []float64 {
	if len(segs) == 0 {
		if f, ok := toFloat(node); ok {
			return []float64{f}
		}
		return nil
	}
	seg := segs[0]
	rest := segs[1:]

	if strings.HasSuffix(seg, "[]") {
		key := strings.TrimSuffix(seg, "[]")
		m, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		arr, ok := m[key].([]any)
		if !ok {
			return nil
		}
		var out []float64
		for _, el := range arr {
			out = append(out, extractNumbers(el, rest)...)
		}
		return out
	}

	m, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	return extractNumbers(m[seg], rest)
}

// toFloat 把 JSON 反序列化出的数值转 float64。
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	}
	return 0, false
}

// compare 按运算符比较两个数值。
func compare(a float64, op string, b float64) bool {
	switch op {
	case ">":
		return a > b
	case ">=":
		return a >= b
	case "<":
		return a < b
	case "<=":
		return a <= b
	case "==":
		return a == b
	case "!=":
		return a != b
	}
	return false
}

// maxSeverity 取两个严重级别中的较高者（critical > warning > none）。
func maxSeverity(a, b core.MonitorSeverity) core.MonitorSeverity {
	rank := func(s core.MonitorSeverity) int {
		switch s {
		case core.MonitorSeverityCritical:
			return 2
		case core.MonitorSeverityWarning:
			return 1
		default:
			return 0
		}
	}
	if rank(b) > rank(a) {
		return b
	}
	return a
}

// normalizeSeverity 条件未填级别时默认 warning。
func normalizeSeverity(s core.MonitorSeverity) core.MonitorSeverity {
	if s == core.MonitorSeverityWarning || s == core.MonitorSeverityCritical {
		return s
	}
	return core.MonitorSeverityWarning
}

// trimFloat 去掉数值多余的尾零，便于证据文案展示。
func trimFloat(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}
