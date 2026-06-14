// Troubleshoot Flow 报告生成单测：验证证据与采集数据被纳入报告正文。

package monitor

import (
	"strings"
	"testing"
	"time"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
)

func TestBuildTroubleshootReport(t *testing.T) {
	panel := core.MonitorPanel{ID: "p1", Name: "CPU 使用率是否大于 80%"}
	inc := core.Incident{
		Severity:  core.MonitorSeverityWarning,
		Title:     "CPU 异常",
		Evidence:  []string{"host.basic.cpu_usage_percent = 91.2 > 80"},
		StartedAt: time.Now(),
	}
	batch := batchWith(KindHostBasic, "web-01", HostBasicResult{CPUUsagePercent: 91.2})

	tr := BuildTroubleshootReport(panel, inc, batch, nil)

	if !strings.Contains(tr.Title, panel.Name) {
		t.Fatalf("标题应含监控项名: %s", tr.Title)
	}
	if !strings.Contains(tr.Content, "host.basic.cpu_usage_percent = 91.2 > 80") {
		t.Fatalf("正文应含触发证据:\n%s", tr.Content)
	}
	if !strings.Contains(tr.Content, "cpu_usage_percent") {
		t.Fatalf("正文应含当前采集数据:\n%s", tr.Content)
	}
	// 未配置 AI（summarize=nil）时应降级。
	if !strings.Contains(tr.Content, "未配置 AI") {
		t.Fatalf("无 AI 时诊断结论应降级:\n%s", tr.Content)
	}
}

// 无采集数据时报告仍可生成，不 panic。
func TestBuildTroubleshootReport_EmptyBatch(t *testing.T) {
	tr := BuildTroubleshootReport(core.MonitorPanel{Name: "x"}, core.Incident{}, Batch{}, nil)
	if !strings.Contains(tr.Content, "未采集到数据") {
		t.Fatalf("空采集应有提示:\n%s", tr.Content)
	}
}

// 配置了 AI 时，诊断结论段使用 AI 返回，且事实摘要被喂给 AI。
func TestBuildTroubleshootReport_WithAI(t *testing.T) {
	panel := core.MonitorPanel{Name: "CPU", Description: "CPU 监控"}
	inc := core.Incident{Severity: core.MonitorSeverityWarning, Evidence: []string{"cpu>80"}}
	batch := batchWith(KindHostBasic, "web-01", HostBasicResult{CPUUsagePercent: 91.2})

	var gotUser string
	summarize := func(msgs []clients.ChatMessage) (string, error) {
		for _, m := range msgs {
			if m.Role == "user" {
				gotUser = m.Content
			}
		}
		return "根因：CPU 飙高。建议：扩容。", nil
	}

	tr := BuildTroubleshootReport(panel, inc, batch, summarize)
	if !strings.Contains(tr.Content, "根因：CPU 飙高") {
		t.Fatalf("正文应含 AI 诊断结论:\n%s", tr.Content)
	}
	if !strings.Contains(gotUser, "cpu>80") {
		t.Fatalf("应把触发证据喂给 AI: %s", gotUser)
	}
}

// AI 调用失败时降级为提示，不丢事实。
func TestBuildTroubleshootReport_AIFailureDegrades(t *testing.T) {
	summarize := func([]clients.ChatMessage) (string, error) { return "", errTest }
	tr := BuildTroubleshootReport(core.MonitorPanel{Name: "x"}, core.Incident{}, Batch{}, summarize)
	if !strings.Contains(tr.Content, "AI 诊断失败") {
		t.Fatalf("AI 失败应降级提示:\n%s", tr.Content)
	}
}
