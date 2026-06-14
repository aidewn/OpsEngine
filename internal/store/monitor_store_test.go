// MonitorStore 单元测试
// 覆盖分组/监控项 CRUD 往返、按环境与分组过滤、排序、空目录、不存在 ID 的错误语义

package store

import (
	"strings"
	"testing"
	"time"

	"OpsEngine/internal/core"
)

// 构造一个分组样本
func sampleGroup(id, envID string, order int) core.MonitorGroup {
	return core.MonitorGroup{
		ID:            id,
		EnvironmentID: envID,
		Name:          "分组-" + id,
		Description:   "测试分组",
		Order:         order,
		Enabled:       true,
	}
}

// 构造一个监控项样本（带一条数据需求，验证嵌套结构往返）
func samplePanel(id, envID, groupID string) core.MonitorPanel {
	return core.MonitorPanel{
		ID:            id,
		EnvironmentID: envID,
		GroupID:       groupID,
		Name:          "监控项-" + id,
		Description:   "测试监控项",
		Enabled:       true,
		Requirements: []core.DataRequirement{
			{TargetID: "ssh-app", Kind: "host.disk", Params: map[string]any{"mount": "/"}},
		},
		MonitorFlowSummary:      "读取磁盘水位 -> 阈值判断",
		TroubleshootFlowSummary: "定位大目录",
	}
}

// 分组 Save → Get 往返
func TestMonitorStore_GroupRoundTrip(t *testing.T) {
	store := NewMonitorStore(t.TempDir())
	g := sampleGroup("g1", "env-1", 1)
	if err := store.SaveGroup(g); err != nil {
		t.Fatalf("SaveGroup 失败: %v", err)
	}
	got, err := store.GetGroup("g1")
	if err != nil {
		t.Fatalf("GetGroup 失败: %v", err)
	}
	if got.ID != g.ID || got.Name != g.Name || got.EnvironmentID != g.EnvironmentID || !got.Enabled {
		t.Fatalf("分组字段不一致: %+v", got)
	}
}

// 监控项 Save → Get 往返，含嵌套 Requirements
func TestMonitorStore_PanelRoundTrip(t *testing.T) {
	store := NewMonitorStore(t.TempDir())
	p := samplePanel("p1", "env-1", "g1")
	if err := store.SavePanel(p); err != nil {
		t.Fatalf("SavePanel 失败: %v", err)
	}
	got, err := store.GetPanel("p1")
	if err != nil {
		t.Fatalf("GetPanel 失败: %v", err)
	}
	if len(got.Requirements) != 1 {
		t.Fatalf("Requirements 数量错误: %d", len(got.Requirements))
	}
	req := got.Requirements[0]
	if req.TargetID != "ssh-app" || req.Kind != "host.disk" || req.Params["mount"] != "/" {
		t.Fatalf("Requirement 往返不一致: %+v", req)
	}
	if got.MonitorFlowSummary != p.MonitorFlowSummary {
		t.Fatalf("MonitorFlowSummary 不一致: %s", got.MonitorFlowSummary)
	}
}

// ListGroups 只返回指定环境的分组，并按 Order 升序
func TestMonitorStore_ListGroupsFilterAndSort(t *testing.T) {
	store := NewMonitorStore(t.TempDir())
	mustSaveGroup(t, store, sampleGroup("a", "env-1", 2))
	mustSaveGroup(t, store, sampleGroup("b", "env-1", 1))
	mustSaveGroup(t, store, sampleGroup("c", "env-2", 1)) // 其它环境，应被过滤

	groups, err := store.ListGroups("env-1")
	if err != nil {
		t.Fatalf("ListGroups 失败: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("期望 2 个分组，实际 %d", len(groups))
	}
	if groups[0].ID != "b" || groups[1].ID != "a" {
		t.Fatalf("分组未按 Order 升序: %s, %s", groups[0].ID, groups[1].ID)
	}
}

// ListPanels 支持按环境过滤，以及按分组进一步过滤
func TestMonitorStore_ListPanelsFilter(t *testing.T) {
	store := NewMonitorStore(t.TempDir())
	mustSavePanel(t, store, samplePanel("p1", "env-1", "g1"))
	mustSavePanel(t, store, samplePanel("p2", "env-1", "g2"))
	mustSavePanel(t, store, samplePanel("p3", "env-2", "g1")) // 其它环境

	// 仅按环境过滤
	all, err := store.ListPanels("env-1", "")
	if err != nil {
		t.Fatalf("ListPanels(env) 失败: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("期望 env-1 下 2 个监控项，实际 %d", len(all))
	}

	// 环境 + 分组过滤
	g1, err := store.ListPanels("env-1", "g1")
	if err != nil {
		t.Fatalf("ListPanels(env, group) 失败: %v", err)
	}
	if len(g1) != 1 || g1[0].ID != "p1" {
		t.Fatalf("分组过滤结果错误: %+v", g1)
	}
}

// 删除后再 Get 必须报错
func TestMonitorStore_DeletePanel(t *testing.T) {
	store := NewMonitorStore(t.TempDir())
	mustSavePanel(t, store, samplePanel("p1", "env-1", "g1"))
	if err := store.DeletePanel("p1"); err != nil {
		t.Fatalf("DeletePanel 失败: %v", err)
	}
	if _, err := store.GetPanel("p1"); err == nil {
		t.Fatal("删除后 GetPanel 仍成功")
	}
}

// 删除不存在的监控项不报错（与 os.Remove + IsNotExist 兜底一致）
func TestMonitorStore_DeleteMissingIsNoop(t *testing.T) {
	store := NewMonitorStore(t.TempDir())
	if err := store.DeletePanel("missing"); err != nil {
		t.Fatalf("删除不存在的监控项应为 no-op，实际报错: %v", err)
	}
}

// 不存在 ID 的 Get 返回带提示的错误
func TestMonitorStore_GetNotFound(t *testing.T) {
	store := NewMonitorStore(t.TempDir())
	if _, err := store.GetGroup("missing"); err == nil || !strings.Contains(err.Error(), "监控分组未找到") {
		t.Fatalf("GetGroup 错误语义不符: %v", err)
	}
	if _, err := store.GetPanel("missing"); err == nil || !strings.Contains(err.Error(), "监控项未找到") {
		t.Fatalf("GetPanel 错误语义不符: %v", err)
	}
}

// 空目录 List 返回非 nil 的空切片且不报错。
// 必须是非 nil：Wails 会把 nil 切片序列化成 JSON null，前端按数组访问会崩溃。
func TestMonitorStore_ListEmpty(t *testing.T) {
	store := NewMonitorStore(t.TempDir())
	groups, err := store.ListGroups("env-1")
	if err != nil || groups == nil || len(groups) != 0 {
		t.Fatalf("空目录 ListGroups 异常: err=%v nil=%v len=%d", err, groups == nil, len(groups))
	}
	panels, err := store.ListPanels("env-1", "")
	if err != nil || panels == nil || len(panels) != 0 {
		t.Fatalf("空目录 ListPanels 异常: err=%v nil=%v len=%d", err, panels == nil, len(panels))
	}
}

// PanelState 持久化往返：含指针时间字段，验证 TOML 编解码不丢字段。
func TestMonitorStore_PanelStateRoundTrip(t *testing.T) {
	store := NewMonitorStore(t.TempDir())

	// 不存在时返回 (zero, false, nil)
	if _, ok, err := store.GetPanelState("p1"); err != nil || ok {
		t.Fatalf("未保存时应返回 ok=false: ok=%v err=%v", ok, err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	st := core.PanelState{
		PanelID:       "p1",
		Status:        core.MonitorStatusAbnormal,
		Severity:      core.MonitorSeverityCritical,
		Summary:       "磁盘超阈值",
		LastCheckedAt: &now,
		HasHistory:    true,
	}
	if err := store.SavePanelState(st); err != nil {
		t.Fatalf("SavePanelState 失败: %v", err)
	}
	got, ok, err := store.GetPanelState("p1")
	if err != nil || !ok {
		t.Fatalf("GetPanelState 失败: ok=%v err=%v", ok, err)
	}
	if got.Status != core.MonitorStatusAbnormal || got.Severity != core.MonitorSeverityCritical || !got.HasHistory {
		t.Fatalf("状态字段不一致: %+v", got)
	}
	if got.LastCheckedAt == nil || !got.LastCheckedAt.Equal(now) {
		t.Fatalf("LastCheckedAt 往返不一致: %v vs %v", got.LastCheckedAt, now)
	}

	if err := store.DeletePanelState("p1"); err != nil {
		t.Fatalf("DeletePanelState 失败: %v", err)
	}
	if _, ok, _ := store.GetPanelState("p1"); ok {
		t.Fatal("删除后仍能读到状态")
	}
}

// Incident 持久化往返 + 按环境过滤、按开始时间倒序。
func TestMonitorStore_Incidents(t *testing.T) {
	store := NewMonitorStore(t.TempDir())
	base := time.Now().UTC().Truncate(time.Second)
	resolved := base.Add(time.Hour)
	mustSaveIncident(t, store, core.Incident{
		ID: "i1", EnvironmentID: "env-1", PanelID: "p1", Status: core.IncidentStatusOpen,
		Severity: core.MonitorSeverityWarning, Title: "旧", Evidence: []string{"e1"}, StartedAt: base,
	})
	mustSaveIncident(t, store, core.Incident{
		ID: "i2", EnvironmentID: "env-1", PanelID: "p2", Status: core.IncidentStatusResolved,
		Title: "新", StartedAt: base.Add(time.Minute), ResolvedAt: &resolved,
	})
	mustSaveIncident(t, store, core.Incident{ID: "i3", EnvironmentID: "env-2", StartedAt: base})

	list, err := store.ListIncidents("env-1")
	if err != nil {
		t.Fatalf("ListIncidents 失败: %v", err)
	}
	if len(list) != 2 || list[0].ID != "i2" {
		t.Fatalf("应返回 env-1 的 2 条且最新在前: %+v", list)
	}
	got, err := store.GetIncident("i1")
	if err != nil || len(got.Evidence) != 1 || got.Evidence[0] != "e1" {
		t.Fatalf("Incident 往返不一致: %+v err=%v", got, err)
	}
	if got2, _ := store.GetIncident("i2"); got2.ResolvedAt == nil || !got2.ResolvedAt.Equal(resolved) {
		t.Fatalf("ResolvedAt 往返不一致: %+v", got2)
	}
}

// MonitorReport 持久化往返 + 按环境过滤。
func TestMonitorStore_Reports(t *testing.T) {
	store := NewMonitorStore(t.TempDir())
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.SaveReport(core.MonitorReport{
		ID: "r1", EnvironmentID: "env-1", PanelID: "p1", IncidentID: "i1",
		Title: "诊断", Summary: "摘要", Content: "# 正文", CreatedAt: now,
	}); err != nil {
		t.Fatalf("SaveReport 失败: %v", err)
	}
	_ = store.SaveReport(core.MonitorReport{ID: "r2", EnvironmentID: "env-2", CreatedAt: now})

	list, err := store.ListReports("env-1")
	if err != nil || len(list) != 1 || list[0].ID != "r1" {
		t.Fatalf("ListReports 过滤错误: %+v err=%v", list, err)
	}
	got, err := store.GetReport("r1")
	if err != nil || got.Content != "# 正文" || got.IncidentID != "i1" {
		t.Fatalf("Report 往返不一致: %+v err=%v", got, err)
	}
}

func mustSaveIncident(t *testing.T, s *MonitorStore, inc core.Incident) {
	t.Helper()
	if err := s.SaveIncident(inc); err != nil {
		t.Fatalf("SaveIncident(%s) 失败: %v", inc.ID, err)
	}
}

func mustSaveGroup(t *testing.T, s *MonitorStore, g core.MonitorGroup) {
	t.Helper()
	if err := s.SaveGroup(g); err != nil {
		t.Fatalf("SaveGroup(%s) 失败: %v", g.ID, err)
	}
}

func mustSavePanel(t *testing.T, s *MonitorStore, p core.MonitorPanel) {
	t.Helper()
	if err := s.SavePanel(p); err != nil {
		t.Fatalf("SavePanel(%s) 失败: %v", p.ID, err)
	}
}
