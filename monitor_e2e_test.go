// 监控端到端验证（http.health）：配条件 → tick 检出异常 → 建 Incident → 诊断(真实 AI) → 看报告 → 恢复。
// 默认跳过；设 OPSENGINE_E2E=1 运行（会真实调用 LLM，需 data/settings/ai.toml 配好 Key）。

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"OpsEngine/internal/core"
	"OpsEngine/internal/monitor"
	"OpsEngine/internal/store"
)

func TestMonitorHTTPHealthE2E(t *testing.T) {
	if os.Getenv("OPSENGINE_E2E") == "" {
		t.Skip("设置 OPSENGINE_E2E=1 运行真实端到端验证（会调用真实 LLM）")
	}

	// 被监控的 HTTP 服务：初始返回 500（异常），后续可切回 200（恢复）。
	var status int32 = 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(atomic.LoadInt32(&status)))
	}))
	defer srv.Close()

	dir := t.TempDir()
	envDir := filepath.Join(dir, "env")
	if err := os.MkdirAll(envDir, 0755); err != nil {
		t.Fatal(err)
	}
	a := &App{
		environmentStore:      store.NewEnvironmentStore(envDir),
		monitorStore:          store.NewMonitorStore(filepath.Join(dir, "monitor")),
		monitorDiagnosisGuard: monitor.NewKeyedGuard(),
	}

	// 1) 建环境 / 分组 / 监控项
	envID, err := a.CreateEnvironment("e2e", "")
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	groupID, err := a.CreateMonitorGroup(envID, "Web 服务", "")
	if err != nil {
		t.Fatalf("CreateMonitorGroup: %v", err)
	}
	panelID, err := a.CreateMonitorPanel(envID, groupID, "HTTP 健康检查是否失败", "探测入口健康检查状态码")
	if err != nil {
		t.Fatalf("CreateMonitorPanel: %v", err)
	}

	// 2) 配数据需求(http.health) + 判断条件(status_code != 200)
	panel, err := a.GetMonitorPanel(panelID)
	if err != nil {
		t.Fatalf("GetMonitorPanel: %v", err)
	}
	panel.Requirements = []core.DataRequirement{
		{Kind: "http.health", Params: map[string]any{"url": srv.URL}},
	}
	panel.Conditions = []core.MonitorCondition{
		{Kind: "http.health", Field: "status_code", Op: "!=", Value: 200, Severity: core.MonitorSeverityWarning},
	}
	if err := a.UpdateMonitorPanel(panel); err != nil {
		t.Fatalf("UpdateMonitorPanel: %v", err)
	}

	// 3) 跑一轮 tick（= 后台调度器的一次自动采集判断）→ 应检出异常并建 Incident
	ov, err := a.RunMonitorTick(envID)
	if err != nil {
		t.Fatalf("RunMonitorTick: %v", err)
	}
	st := findState(t, ov, panelID)
	if st.Status != core.MonitorStatusAbnormal {
		t.Fatalf("期望 abnormal，实际 %s（%s）", st.Status, st.Summary)
	}
	if st.CurrentIncident == "" || len(ov.Incidents) == 0 {
		t.Fatalf("应创建 Incident，states=%+v incidents=%+v", st, ov.Incidents)
	}
	t.Logf("✓ 检出异常: %s ；证据=%v", st.Summary, ov.Incidents[0].Evidence)

	// 4) 触发诊断（异步，真实 AI）→ 轮询直到报告生成且诊断收尾
	if err := a.RunPanelDiagnosis(panelID); err != nil {
		t.Fatalf("RunPanelDiagnosis: %v", err)
	}
	var rep core.MonitorReport
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		o, err := a.GetMonitorOverview(envID)
		if err != nil {
			t.Fatalf("GetMonitorOverview: %v", err)
		}
		s := findState(t, o, panelID)
		if len(o.Reports) > 0 && s.LastReportID != "" && s.Status != core.MonitorStatusDiagnosing {
			rep = o.Reports[0]
			break
		}
		time.Sleep(2 * time.Second)
	}
	if rep.ID == "" {
		t.Fatal("诊断报告未在超时内生成")
	}
	t.Logf("✓ 诊断报告已生成: %s\n========== 报告正文 ==========\n%s\n========== 正文结束 ==========", rep.Title, rep.Content)

	// 5) 恢复：服务返回 200，再跑一轮 → 状态转「正常（有异常历史）」，Incident 关闭
	atomic.StoreInt32(&status, 200)
	ov2, err := a.RunMonitorTick(envID)
	if err != nil {
		t.Fatalf("recovery RunMonitorTick: %v", err)
	}
	st2 := findState(t, ov2, panelID)
	if st2.Status != core.MonitorStatusHistory {
		t.Fatalf("恢复后应为 history，实际 %s", st2.Status)
	}
	t.Logf("✓ 恢复后状态: %s（有异常历史，报告仍可回看）", st2.Status)
}

func findState(t *testing.T, ov core.MonitorOverview, panelID string) core.PanelState {
	t.Helper()
	for _, s := range ov.States {
		if s.PanelID == panelID {
			return s
		}
	}
	t.Fatalf("概览中未找到监控项 %s 的状态", panelID)
	return core.PanelState{}
}
