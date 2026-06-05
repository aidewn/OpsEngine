// 巡检报告事实提取与 Markdown 渲染单测。

package report

import (
	"strings"
	"testing"
	"time"

	"OpsEngine/internal/core"
)

// sampleData 构造一份典型巡检工作流 + 执行记录，覆盖成功/失败混合场景。
func sampleData() (core.EnvironmentDef, core.WorkflowDef, core.ExecutionRecord) {
	env := core.EnvironmentDef{ID: "env-1", Name: "生产环境"}
	wf := core.WorkflowDef{
		ID: "wf-1", Name: "测试巡检",
		Nodes: []core.NodeInstance{
			{InstanceID: "ready", TypeID: "system_ready"},
			{InstanceID: "ssh", TypeID: "env_connect_ssh"},
			{InstanceID: "n-os", TypeID: "linux_exec_command", Config: map[string]any{
				"title": "系统信息", "command": "uname -a",
			}},
			{InstanceID: "n-df", TypeID: "linux_exec_command", Config: map[string]any{
				"title": "磁盘", "command": "df -h",
			}},
			// 无 title 的 linux_exec_command 不计入巡检项
			{InstanceID: "n-extra", TypeID: "linux_exec_command", Config: map[string]any{
				"command": "echo extra",
			}},
		},
	}
	finishedAt := time.Date(2026, 6, 5, 10, 5, 0, 0, time.UTC)
	rec := core.ExecutionRecord{
		ID: "exec-1", WorkflowID: "wf-1", Status: core.WorkflowStatusFailed,
		StartedAt:  time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC),
		FinishedAt: &finishedAt,
		RootFrame: core.FrameState{
			NodeStates: map[string]core.NodeState{
				"n-os": core.NodeStateSuccess,
				"n-df": core.NodeStateFailed,
			},
			NodeLogs: map[string][]core.LogEntry{
				"n-os": {{Level: "info", Message: "stdout: Linux web-01 5.10.0"}},
				"n-df": {{Level: "error", Message: "exit_code=1"}, {Level: "info", Message: "stdout:"}},
			},
		},
	}
	return env, wf, rec
}

// TestExtractFactsSkipsUntitled 验证无 title 的节点不进入巡检项。
func TestExtractFactsSkipsUntitled(t *testing.T) {
	env, wf, rec := sampleData()
	r := ExtractFacts(env, wf, rec)
	if len(r.Items) != 2 {
		t.Fatalf("expected 2 items (n-os + n-df), got %d", len(r.Items))
	}
	for _, it := range r.Items {
		if it.NodeID == "n-extra" {
			t.Fatal("无 title 节点不应进入巡检项")
		}
	}
}

// TestExtractFactsCarriesState 验证状态、命令、日志都被正确带入。
func TestExtractFactsCarriesState(t *testing.T) {
	env, wf, rec := sampleData()
	r := ExtractFacts(env, wf, rec)
	if r.Items[0].State != core.NodeStateSuccess || r.Items[1].State != core.NodeStateFailed {
		t.Fatalf("state 提取异常: %#v", r.Items)
	}
	if !strings.Contains(r.Items[0].Logs[0], "Linux web-01") {
		t.Fatalf("日志未带入: %#v", r.Items[0].Logs)
	}
	if !r.HasFailures() {
		t.Fatal("应识别为有失败项")
	}
}

// TestRenderIncludesKeyParts 验证渲染产物包含关键段落。
func TestRenderIncludesKeyParts(t *testing.T) {
	env, wf, rec := sampleData()
	r := ExtractFacts(env, wf, rec)
	md := Render(r, "")
	for _, want := range []string{
		"# 巡检报告：测试巡检",
		"## 巡检概览",
		"## 检查结果",
		"## 风险与建议",
		"系统信息", "磁盘",
		"✅ 成功", "❌ 失败",
		"⚠️", // 失败提示
		"未启用 LLM 风险分析",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("渲染缺少 %q\n%s", want, md)
		}
	}
}

// TestRenderWithRiskSection 验证 LLM 风险段被注入，且默认提示不出现。
func TestRenderWithRiskSection(t *testing.T) {
	env, wf, rec := sampleData()
	r := ExtractFacts(env, wf, rec)
	md := Render(r, "- 磁盘根分区使用率 95%，建议立即扩容")
	if !strings.Contains(md, "磁盘根分区使用率") {
		t.Fatal("LLM 风险段未注入")
	}
	if strings.Contains(md, "未启用 LLM 风险分析") {
		t.Fatal("有风险段时不应再显示默认占位")
	}
}

// TestFormatLogsTruncatesLong 验证单条超长日志会被截断。
func TestFormatLogsTruncatesLong(t *testing.T) {
	long := strings.Repeat("x", 2000)
	out := formatLogs([]core.LogEntry{{Level: "info", Message: long}})
	if len(out) != 1 || !strings.HasSuffix(out[0], "...(截断)") {
		t.Fatalf("超长日志未截断: %s", out[0])
	}
}
