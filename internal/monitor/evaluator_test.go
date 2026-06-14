// Monitor Flow 与状态机单测：阈值判断、数组展开、严重级别、四态迁移。

package monitor

import (
	"strings"
	"testing"
	"time"

	"OpsEngine/internal/core"
)

// 构造只含一条结果的 batch（kind+target 对应条件）。
func batchWith(kind, target string, data any) Batch {
	task := CollectionTask{TargetID: target, Kind: kind, key: collectionKey(target, kind, nil)}
	return Batch{task.key: CollectionResult{Task: task, Data: data}}
}

func condPanel(conds ...core.MonitorCondition) core.MonitorPanel {
	return core.MonitorPanel{ID: "p1", Enabled: true, Conditions: conds}
}

// 标量字段阈值命中 → 异常。
func TestEvaluate_ScalarThreshold(t *testing.T) {
	panel := condPanel(core.MonitorCondition{
		Kind: KindHostBasic, Field: "cpu_usage_percent", Op: ">", Value: 80,
		Severity: core.MonitorSeverityWarning,
	})
	batch := batchWith(KindHostBasic, "", HostBasicResult{CPUUsagePercent: 91.2})

	res := Evaluate(panel, batch)
	if res.Status != core.MonitorStatusAbnormal || res.Severity != core.MonitorSeverityWarning {
		t.Fatalf("应判为 warning 异常: %+v", res)
	}
	if !res.ShouldTroubleshoot || len(res.Evidence) != 1 {
		t.Fatalf("异常应建议排查并带证据: %+v", res)
	}
}

// 阈值未命中 → 正常。
func TestEvaluate_ScalarNormal(t *testing.T) {
	panel := condPanel(core.MonitorCondition{Kind: KindHostBasic, Field: "cpu_usage_percent", Op: ">", Value: 80})
	batch := batchWith(KindHostBasic, "", HostBasicResult{CPUUsagePercent: 12.5})
	if res := Evaluate(panel, batch); res.Status != core.MonitorStatusNormal {
		t.Fatalf("应判为正常: %+v", res)
	}
}

// 数组展开：任一文件系统超阈值即异常。
func TestEvaluate_ArrayExpansion(t *testing.T) {
	panel := condPanel(core.MonitorCondition{
		Kind: KindHostDisk, Field: "filesystems[].use_percent", Op: ">=", Value: 85,
		Severity: core.MonitorSeverityCritical,
	})
	batch := batchWith(KindHostDisk, "", HostDiskResult{Filesystems: []DiskFilesystem{
		{Mount: "/", UsePercent: 40},
		{Mount: "/data", UsePercent: 92},
	}})
	res := Evaluate(panel, batch)
	if res.Status != core.MonitorStatusAbnormal || res.Severity != core.MonitorSeverityCritical {
		t.Fatalf("任一分区超阈值应判 critical 异常: %+v", res)
	}
}

// 多条件取最高严重级别。
func TestEvaluate_MaxSeverity(t *testing.T) {
	panel := condPanel(
		core.MonitorCondition{Kind: KindHostBasic, Field: "cpu_usage_percent", Op: ">", Value: 10, Severity: core.MonitorSeverityWarning},
		core.MonitorCondition{Kind: KindHostBasic, Field: "mem_usage_percent", Op: ">", Value: 10, Severity: core.MonitorSeverityCritical},
	)
	batch := batchWith(KindHostBasic, "", HostBasicResult{CPUUsagePercent: 50, MemUsagePercent: 50})
	if res := Evaluate(panel, batch); res.Severity != core.MonitorSeverityCritical {
		t.Fatalf("应取最高级别 critical: %+v", res)
	}
}

// 采集失败（如目标不可达）应判为异常，并把错误写进证据——不能静默显示正常。
func TestEvaluate_CollectErrorIsAbnormal(t *testing.T) {
	panel := condPanel(core.MonitorCondition{
		Kind: KindHostBasic, Field: "cpu_usage_percent", Op: ">", Value: 80,
		Severity: core.MonitorSeverityWarning,
	})
	task := CollectionTask{Kind: KindHostBasic, key: collectionKey("", KindHostBasic, nil)}
	batch := Batch{task.key: CollectionResult{Task: task, Err: errTest}}
	res := Evaluate(panel, batch)
	if res.Status != core.MonitorStatusAbnormal {
		t.Fatalf("采集失败应判异常: %+v", res)
	}
	if len(res.Evidence) == 0 || !strings.Contains(res.Evidence[0], "采集失败") {
		t.Fatalf("证据应说明采集失败: %+v", res.Evidence)
	}
}

// 条件缺少对应采集数据时也判为异常（无法判断本身是问题）。
func TestEvaluate_MissingDataIsAbnormal(t *testing.T) {
	panel := condPanel(core.MonitorCondition{Kind: KindHostBasic, Field: "cpu_usage_percent", Op: ">", Value: 80})
	if res := Evaluate(panel, Batch{}); res.Status != core.MonitorStatusAbnormal {
		t.Fatalf("缺数据应判异常: %+v", res)
	}
}

var errTest = &testErr{}

type testErr struct{}

func (*testErr) Error() string { return "boom" }

// 无条件视为正常。
func TestEvaluate_NoConditions(t *testing.T) {
	if res := Evaluate(core.MonitorPanel{ID: "p1"}, Batch{}); res.Status != core.MonitorStatusNormal {
		t.Fatalf("无条件应正常: %+v", res)
	}
}

// 状态机：正常 → 异常。
func TestNextPanelState_NormalToAbnormal(t *testing.T) {
	prev := core.PanelState{PanelID: "p1", Status: core.MonitorStatusNormal}
	result := FlowResult{Status: core.MonitorStatusAbnormal, Severity: core.MonitorSeverityWarning, Summary: "CPU 高"}
	next := NextPanelState(prev, "p1", result, Thresholds{}, time.Now())
	if next.Status != core.MonitorStatusAbnormal || !next.HasHistory || next.LastCheckedAt == nil {
		t.Fatalf("应迁移到 abnormal 并置 HasHistory: %+v", next)
	}
}

// 状态机：异常恢复 → 正常（有异常历史）。
func TestNextPanelState_AbnormalToHistory(t *testing.T) {
	prev := core.PanelState{PanelID: "p1", Status: core.MonitorStatusAbnormal, HasHistory: true}
	result := FlowResult{Status: core.MonitorStatusNormal}
	next := NextPanelState(prev, "p1", result, Thresholds{}, time.Now())
	if next.Status != core.MonitorStatusHistory || next.Severity != core.MonitorSeverityNone {
		t.Fatalf("恢复后应为 history: %+v", next)
	}
}

// 状态机：从未异常的正常项保持纯 normal。
func TestNextPanelState_StaysNormal(t *testing.T) {
	prev := core.PanelState{PanelID: "p1", Status: core.MonitorStatusNormal}
	next := NextPanelState(prev, "p1", FlowResult{Status: core.MonitorStatusNormal}, Thresholds{}, time.Now())
	if next.Status != core.MonitorStatusNormal || next.HasHistory {
		t.Fatalf("应保持纯 normal: %+v", next)
	}
}

// 防抖：异常阈值=3，连续 2 次异常不翻转，第 3 次才翻为 abnormal。
func TestNextPanelState_AbnormalDebounce(t *testing.T) {
	th := Thresholds{Abnormal: 3, Recovery: 1}
	abnormal := FlowResult{Status: core.MonitorStatusAbnormal, Severity: core.MonitorSeverityWarning}
	st := core.PanelState{PanelID: "p1", Status: core.MonitorStatusNormal}

	st = NextPanelState(st, "p1", abnormal, th, time.Now())
	if st.Status != core.MonitorStatusNormal || st.ConsecutiveAbnormal != 1 {
		t.Fatalf("第 1 次异常应仍正常、计数 1: %+v", st)
	}
	st = NextPanelState(st, "p1", abnormal, th, time.Now())
	if st.Status != core.MonitorStatusNormal || st.ConsecutiveAbnormal != 2 {
		t.Fatalf("第 2 次异常应仍正常、计数 2: %+v", st)
	}
	st = NextPanelState(st, "p1", abnormal, th, time.Now())
	if st.Status != core.MonitorStatusAbnormal || st.ConsecutiveAbnormal != 3 {
		t.Fatalf("第 3 次异常应翻为 abnormal: %+v", st)
	}
}

// 防抖：异常累计中途出现一次正常，计数清零，不会告警。
func TestNextPanelState_AbnormalResetByNormal(t *testing.T) {
	th := Thresholds{Abnormal: 3}
	abnormal := FlowResult{Status: core.MonitorStatusAbnormal, Severity: core.MonitorSeverityWarning}
	normal := FlowResult{Status: core.MonitorStatusNormal}

	st := core.PanelState{PanelID: "p1", Status: core.MonitorStatusNormal}
	st = NextPanelState(st, "p1", abnormal, th, time.Now())
	st = NextPanelState(st, "p1", abnormal, th, time.Now())
	st = NextPanelState(st, "p1", normal, th, time.Now()) // 打断
	if st.ConsecutiveAbnormal != 0 || st.Status != core.MonitorStatusNormal {
		t.Fatalf("正常应清零异常计数: %+v", st)
	}
	st = NextPanelState(st, "p1", abnormal, th, time.Now())
	if st.Status != core.MonitorStatusNormal || st.ConsecutiveAbnormal != 1 {
		t.Fatalf("打断后重新累计应从 1 开始: %+v", st)
	}
}

// 防抖：恢复阈值=2，异常后单次正常不解除，连续 2 次正常才转 history。
func TestNextPanelState_RecoveryDebounce(t *testing.T) {
	th := Thresholds{Abnormal: 1, Recovery: 2}
	normal := FlowResult{Status: core.MonitorStatusNormal}
	prev := core.PanelState{PanelID: "p1", Status: core.MonitorStatusAbnormal, HasHistory: true}

	st := NextPanelState(prev, "p1", normal, th, time.Now())
	if st.Status != core.MonitorStatusAbnormal || st.ConsecutiveNormal != 1 {
		t.Fatalf("恢复第 1 次应维持 abnormal: %+v", st)
	}
	st = NextPanelState(st, "p1", normal, th, time.Now())
	if st.Status != core.MonitorStatusHistory {
		t.Fatalf("恢复第 2 次应转 history: %+v", st)
	}
}
