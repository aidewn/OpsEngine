// 监控 Wails 适配层（Phase 1：信息架构与静态管理）。
//
// 本文件只做参数校验、ID/时间戳装配，把 CRUD 桥接到 monitorStore；
// 采集、判断、Incident、报告等运行能力属于后续 Phase，此处暂不涉及。
// 概览（GetMonitorOverview）当前返回真实的分组与监控项，并为每个监控项合成
// 默认 normal 状态——Monitor Flow（Phase 3）落地前，监控项一律视为正常。

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"OpsEngine/internal/core"
	"OpsEngine/internal/monitor"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// monitorCollectionTimeout 一轮采集的总超时；单个 SSH 命令另有更短的硬超时。
const monitorCollectionTimeout = 60 * time.Second

// minMonitorIntervalSeconds 自动监控采集间隔下限，避免过于频繁地连服务器。
const minMonitorIntervalSeconds = 10

// errMonitorTickBusy 表示该环境已有一轮 tick 在执行，本次跳过（非错误，用于幂等）。
var errMonitorTickBusy = errors.New("该环境正在检查中")

// ── 监控源 CRUD ────────────────────────────────────────

// ListMonitorSources 列出某环境下的监控源；敏感字段会脱敏返回。
func (a *App) ListMonitorSources(environmentID string) ([]core.MonitorSource, error) {
	if a.monitorStore == nil {
		return []core.MonitorSource{}, nil
	}
	if err := a.assertEnvironmentExists(environmentID); err != nil {
		return nil, err
	}
	sources, err := a.monitorStore.ListSources(environmentID)
	if err != nil {
		return nil, err
	}
	return sanitizeMonitorSources(sources), nil
}

// GetMonitorSource 按 ID 读取监控源；敏感字段会脱敏返回。
func (a *App) GetMonitorSource(id string) (core.MonitorSource, error) {
	if a.monitorStore == nil {
		return core.MonitorSource{}, errors.New("监控存储未初始化")
	}
	source, err := a.monitorStore.GetSource(id)
	if err != nil {
		return core.MonitorSource{}, err
	}
	return sanitizeMonitorSource(source), nil
}

// CreateMonitorSource 在指定环境下创建监控源。
func (a *App) CreateMonitorSource(environmentID, name, kind string, config map[string]any) (string, error) {
	if a.monitorStore == nil {
		return "", errors.New("监控存储未初始化")
	}
	if err := a.assertEnvironmentExists(environmentID); err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	kind = strings.TrimSpace(kind)
	if name == "" {
		return "", errors.New("监控源名称不能为空")
	}
	if err := validateMonitorSourceKind(kind); err != nil {
		return "", err
	}
	now := time.Now()
	source := core.MonitorSource{
		ID:            uuid.New().String(),
		EnvironmentID: environmentID,
		Name:          name,
		Kind:          kind,
		Enabled:       true,
		Config:        normalizeSourceConfig(config),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := a.monitorStore.SaveSource(source); err != nil {
		return "", err
	}
	return source.ID, nil
}

// UpdateMonitorSource 整体覆盖更新监控源；脱敏占位符不会覆盖已保存密钥。
func (a *App) UpdateMonitorSource(source core.MonitorSource) error {
	if a.monitorStore == nil {
		return errors.New("监控存储未初始化")
	}
	if source.ID == core.MonitorBuiltinSourceID {
		return errors.New("内置监控源不能编辑")
	}
	existing, err := a.monitorStore.GetSource(source.ID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(source.Name) == "" {
		return errors.New("监控源名称不能为空")
	}
	if err := validateMonitorSourceKind(source.Kind); err != nil {
		return err
	}
	source.EnvironmentID = existing.EnvironmentID
	source.CreatedAt = existing.CreatedAt
	source.UpdatedAt = time.Now()
	source.Config = mergeSourceSecretConfig(existing.Config, normalizeSourceConfig(source.Config))
	return a.monitorStore.SaveSource(source)
}

// DeleteMonitorSource 删除监控源；仍被监控项引用时拒绝删除。
func (a *App) DeleteMonitorSource(id string) error {
	if a.monitorStore == nil {
		return errors.New("监控存储未初始化")
	}
	if id == core.MonitorBuiltinSourceID {
		return errors.New("内置监控源不能删除")
	}
	source, err := a.monitorStore.GetSource(id)
	if err != nil {
		return err
	}
	panels, err := a.monitorStore.ListPanels(source.EnvironmentID, "")
	if err != nil {
		return err
	}
	for _, panel := range panels {
		for _, req := range panel.Requirements {
			if normalizeMonitorSourceID(req.SourceID) == id {
				return fmt.Errorf("监控源仍被监控项引用: %s", panel.Name)
			}
		}
		for _, cond := range panel.Conditions {
			if normalizeMonitorSourceID(cond.SourceID) == id {
				return fmt.Errorf("监控源仍被监控项引用: %s", panel.Name)
			}
		}
	}
	return a.monitorStore.DeleteSource(id)
}

// TestMonitorSource 测试监控源配置是否可用；不保存入参。
func (a *App) TestMonitorSource(source core.MonitorSource) error {
	if err := a.assertEnvironmentExists(source.EnvironmentID); err != nil {
		return err
	}
	switch source.Kind {
	case core.MonitorSourceKindPrometheus:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return monitor.TestPrometheusSource(ctx, source)
	default:
		return fmt.Errorf("不支持测试该监控源类型: %s", source.Kind)
	}
}

// ── 分组 CRUD ───────────────────────────────────────────

// ListMonitorGroups 列出某环境下的监控分组。
func (a *App) ListMonitorGroups(environmentID string) ([]core.MonitorGroup, error) {
	if a.monitorStore == nil {
		return []core.MonitorGroup{}, nil
	}
	if strings.TrimSpace(environmentID) == "" {
		return nil, errors.New("environment_id 不能为空")
	}
	return a.monitorStore.ListGroups(environmentID)
}

// CreateMonitorGroup 在指定环境下创建分组，返回生成的 ID。
func (a *App) CreateMonitorGroup(environmentID, name, description string) (string, error) {
	if a.monitorStore == nil {
		return "", errors.New("监控存储未初始化")
	}
	if err := a.assertEnvironmentExists(environmentID); err != nil {
		return "", err
	}
	if strings.TrimSpace(name) == "" {
		return "", errors.New("分组名称不能为空")
	}
	now := time.Now()
	group := core.MonitorGroup{
		ID:            uuid.New().String(),
		EnvironmentID: environmentID,
		Name:          strings.TrimSpace(name),
		Description:   description,
		Enabled:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := a.monitorStore.SaveGroup(group); err != nil {
		return "", err
	}
	return group.ID, nil
}

// UpdateMonitorGroup 整体覆盖更新分组，保留 CreatedAt、刷新 UpdatedAt。
func (a *App) UpdateMonitorGroup(group core.MonitorGroup) error {
	if a.monitorStore == nil {
		return errors.New("监控存储未初始化")
	}
	existing, err := a.monitorStore.GetGroup(group.ID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(group.Name) == "" {
		return errors.New("分组名称不能为空")
	}
	group.CreatedAt = existing.CreatedAt
	group.UpdatedAt = time.Now()
	return a.monitorStore.SaveGroup(group)
}

// DeleteMonitorGroup 删除分组。分组下仍有监控项时拒绝删除，避免产生孤儿监控项。
func (a *App) DeleteMonitorGroup(id string) error {
	if a.monitorStore == nil {
		return errors.New("监控存储未初始化")
	}
	group, err := a.monitorStore.GetGroup(id)
	if err != nil {
		return err
	}
	panels, err := a.monitorStore.ListPanels(group.EnvironmentID, id)
	if err != nil {
		return err
	}
	if len(panels) > 0 {
		return fmt.Errorf("分组下仍有 %d 个监控项，请先移除或删除", len(panels))
	}
	return a.monitorStore.DeleteGroup(id)
}

// ── 监控项 CRUD ─────────────────────────────────────────

// ListMonitorPanels 列出某环境（可选指定分组）下的监控项。
func (a *App) ListMonitorPanels(environmentID, groupID string) ([]core.MonitorPanel, error) {
	if a.monitorStore == nil {
		return []core.MonitorPanel{}, nil
	}
	if strings.TrimSpace(environmentID) == "" {
		return nil, errors.New("environment_id 不能为空")
	}
	return a.monitorStore.ListPanels(environmentID, groupID)
}

// GetMonitorPanel 按 ID 获取监控项详情。
func (a *App) GetMonitorPanel(id string) (core.MonitorPanel, error) {
	if a.monitorStore == nil {
		return core.MonitorPanel{}, errors.New("监控存储未初始化")
	}
	return a.monitorStore.GetPanel(id)
}

// CreateMonitorPanel 在指定环境的分组下创建监控项，返回生成的 ID。
// 校验环境与分组存在，且分组确属该环境，避免跨环境挂载。
func (a *App) CreateMonitorPanel(environmentID, groupID, name, description string) (string, error) {
	if a.monitorStore == nil {
		return "", errors.New("监控存储未初始化")
	}
	if err := a.assertEnvironmentExists(environmentID); err != nil {
		return "", err
	}
	group, err := a.monitorStore.GetGroup(groupID)
	if err != nil {
		return "", err
	}
	if group.EnvironmentID != environmentID {
		return "", errors.New("分组不属于该环境")
	}
	if strings.TrimSpace(name) == "" {
		return "", errors.New("监控项名称不能为空")
	}
	now := time.Now()
	panel := core.MonitorPanel{
		ID:                uuid.New().String(),
		EnvironmentID:     environmentID,
		GroupID:           groupID,
		Name:              strings.TrimSpace(name),
		Description:       description,
		Enabled:           true,
		Requirements:      []core.DataRequirement{},
		Conditions:        []core.MonitorCondition{},
		AbnormalThreshold: 1, // 默认即时翻转，不防抖
		RecoveryThreshold: 1,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := a.monitorStore.SavePanel(panel); err != nil {
		return "", err
	}
	return panel.ID, nil
}

// UpdateMonitorPanel 整体覆盖更新监控项，保留 CreatedAt、刷新 UpdatedAt。
func (a *App) UpdateMonitorPanel(panel core.MonitorPanel) error {
	if a.monitorStore == nil {
		return errors.New("监控存储未初始化")
	}
	existing, err := a.monitorStore.GetPanel(panel.ID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(panel.Name) == "" {
		return errors.New("监控项名称不能为空")
	}
	panel.CreatedAt = existing.CreatedAt
	panel.UpdatedAt = time.Now()
	return a.monitorStore.SavePanel(panel)
}

// DeleteMonitorPanel 删除监控项及其持久化状态。
func (a *App) DeleteMonitorPanel(id string) error {
	if a.monitorStore == nil {
		return errors.New("监控存储未初始化")
	}
	if err := a.monitorStore.DeletePanel(id); err != nil {
		return err
	}
	// 状态是监控项的附属物，一并清理；失败不致命。
	_ = a.monitorStore.DeletePanelState(id)
	return nil
}

// ── 概览 ────────────────────────────────────────────────

// GetMonitorOverview 聚合某环境监控首页所需数据。
// Phase 1 阶段 states 为每个监控项合成的默认 normal 状态，incidents/reports 暂为空；
// Phase 3/4 起改为返回真实运行态。
func (a *App) GetMonitorOverview(environmentID string) (core.MonitorOverview, error) {
	if a.monitorStore == nil {
		return core.MonitorOverview{}, errors.New("监控存储未初始化")
	}
	if err := a.assertEnvironmentExists(environmentID); err != nil {
		return core.MonitorOverview{}, err
	}
	groups, err := a.monitorStore.ListGroups(environmentID)
	if err != nil {
		return core.MonitorOverview{}, err
	}
	panels, err := a.monitorStore.ListPanels(environmentID, "")
	if err != nil {
		return core.MonitorOverview{}, err
	}
	states := make([]core.PanelState, 0, len(panels))
	for _, p := range panels {
		// 读持久化状态；未跑过监控判断的监控项合成默认 normal。
		if st, ok, err := a.monitorStore.GetPanelState(p.ID); err == nil && ok {
			states = append(states, st)
		} else {
			states = append(states, core.PanelState{
				PanelID:  p.ID,
				Status:   core.MonitorStatusNormal,
				Severity: core.MonitorSeverityNone,
				Summary:  "尚未运行监控判断",
			})
		}
	}
	incidents, err := a.monitorStore.ListIncidents(environmentID)
	if err != nil {
		return core.MonitorOverview{}, err
	}
	reports, err := a.monitorStore.ListReports(environmentID)
	if err != nil {
		return core.MonitorOverview{}, err
	}
	sources, err := a.monitorStore.ListSources(environmentID)
	if err != nil {
		return core.MonitorOverview{}, err
	}
	return core.MonitorOverview{
		EnvironmentID: environmentID,
		Sources:       sanitizeMonitorSources(sources),
		Groups:        groups,
		Panels:        panels,
		States:        states,
		Incidents:     incidents,
		Reports:       reports,
	}, nil
}

// RunMonitorTick 手动「立即检查一次」：对环境跑一轮 tick 并返回更新后的概览。
// 与后台调度器共用同一套 tick 逻辑（tickEnvironment）。
func (a *App) RunMonitorTick(environmentID string) (core.MonitorOverview, error) {
	if a.monitorStore == nil {
		return core.MonitorOverview{}, errors.New("监控存储未初始化")
	}
	if err := a.assertEnvironmentExists(environmentID); err != nil {
		return core.MonitorOverview{}, err
	}
	env, err := a.environmentStore.Get(environmentID)
	if err != nil {
		return core.MonitorOverview{}, err
	}
	// 已有一轮在跑（后台调度或上一次手动）：不叠加执行，直接返回当前概览。
	if err := a.tickEnvironment(env); err != nil && !errors.Is(err, errMonitorTickBusy) {
		return core.MonitorOverview{}, err
	}
	return a.GetMonitorOverview(environmentID)
}

// tickEnvironment 跑一轮完整监控：采集 → 逐启用监控项 Monitor Flow 判断 → Incident 维护 → 持久化状态。
// 对应 plan §4.3 的 Plan + Collect + Evaluate；正常采集数据不落盘，只持久化状态（plan §5）。
// 后台调度器与手动「立即检查」均调用此方法。
func (a *App) tickEnvironment(env core.EnvironmentDef) error {
	// 按环境加在途锁：同一环境同时只允许一轮 tick，后台调度与手动「立即检查」互斥，
	// 杜绝上一轮未完又起下一轮导致的重复判断 / 报警风暴。
	if a.monitorTickGuard != nil {
		if !a.monitorTickGuard.TryAcquire(env.ID) {
			return errMonitorTickBusy
		}
		defer a.monitorTickGuard.Release(env.ID)
	}

	panels, err := a.monitorStore.ListPanels(env.ID, "")
	if err != nil {
		return err
	}
	sources, err := a.monitorStore.ListSources(env.ID)
	if err != nil {
		return err
	}
	registry, err := monitor.DefaultRegistry()
	if err != nil {
		return err
	}
	collector := monitor.NewCollector(registry)

	ctx, cancel := context.WithTimeout(context.Background(), monitorCollectionTimeout)
	defer cancel()
	batch := collector.RunCollection(ctx, env, sources, panels)

	now := time.Now()
	for _, p := range panels {
		if !p.Enabled {
			continue
		}
		// 防抖阈值按监控项独立（默认 1 = 即时翻转）。
		th := monitor.Thresholds{Abnormal: p.AbnormalThreshold, Recovery: p.RecoveryThreshold}
		prev, _, _ := a.monitorStore.GetPanelState(p.ID)
		result := monitor.Evaluate(p, batch.Slice(p.Requirements))
		next := monitor.NextPanelState(prev, p.ID, result, th, now)
		if err := a.reconcileIncident(env, p, prev, &next, result, now); err != nil {
			return err
		}
		if err := a.monitorStore.SavePanelState(next); err != nil {
			return fmt.Errorf("保存监控项状态失败: %w", err)
		}
	}
	return nil
}

// ── 调度配置与后台调度回调 ───────────────────────────────

// GetMonitorConfig 读取环境监控调度配置（未配置返回默认：未开启、默认间隔）。
func (a *App) GetMonitorConfig(environmentID string) (core.MonitorConfig, error) {
	if a.monitorStore == nil {
		return core.MonitorConfig{}, errors.New("监控存储未初始化")
	}
	if strings.TrimSpace(environmentID) == "" {
		return core.MonitorConfig{}, errors.New("environment_id 不能为空")
	}
	return a.monitorStore.GetConfig(environmentID)
}

// SetMonitorConfig 设置环境监控调度：是否开启自动监控、采集间隔（秒）。
// 间隔下限 10s（防抖阈值已下放到各监控项，见 MonitorPanel）。保存后下个基准节拍自动生效。
func (a *App) SetMonitorConfig(environmentID string, enabled bool, intervalSeconds int) (core.MonitorConfig, error) {
	if a.monitorStore == nil {
		return core.MonitorConfig{}, errors.New("监控存储未初始化")
	}
	if err := a.assertEnvironmentExists(environmentID); err != nil {
		return core.MonitorConfig{}, err
	}
	if intervalSeconds < minMonitorIntervalSeconds {
		intervalSeconds = minMonitorIntervalSeconds
	}
	cfg := core.MonitorConfig{
		EnvironmentID:   environmentID,
		Enabled:         enabled,
		IntervalSeconds: intervalSeconds,
		UpdatedAt:       time.Now(),
	}
	if err := a.monitorStore.SaveConfig(cfg); err != nil {
		return core.MonitorConfig{}, err
	}
	return cfg, nil
}

// monitorSchedules 供调度器读取：返回当前开启监控的环境及其间隔。
func (a *App) monitorSchedules() []monitor.EnvSchedule {
	if a.monitorStore == nil {
		return nil
	}
	configs, err := a.monitorStore.ListConfigs()
	if err != nil {
		return nil
	}
	var out []monitor.EnvSchedule
	for _, c := range configs {
		if c.Enabled {
			out = append(out, monitor.EnvSchedule{EnvironmentID: c.EnvironmentID, IntervalSeconds: c.IntervalSeconds})
		}
	}
	return out
}

// runScheduledTick 供调度器回调：加载环境并跑一轮 tick，错误只记日志（后台静默重试下一轮）。
func (a *App) runScheduledTick(environmentID string) {
	env, err := a.environmentStore.Get(environmentID)
	if err != nil {
		zap.L().Warn("调度 tick 加载环境失败", zap.String("env", environmentID), zap.Error(err))
		return
	}
	if err := a.tickEnvironment(env); err != nil && !errors.Is(err, errMonitorTickBusy) {
		zap.L().Warn("调度 tick 执行失败", zap.String("env", environmentID), zap.Error(err))
	}
}

// reconcileIncident 依据本轮判断维护 Incident 生命周期，并把关联回填进 next 状态：
//   - 异常且当前无未结事件 → 新建 Incident（open）；
//   - 异常且已有事件 → 沿用（next 已从 prev 继承 CurrentIncident）；
//   - 恢复且有未结事件 → 标记 resolved 并清空 CurrentIncident（LastReportID 保留供回看）。
func (a *App) reconcileIncident(
	env core.EnvironmentDef,
	panel core.MonitorPanel,
	prev core.PanelState,
	next *core.PanelState,
	result monitor.FlowResult,
	now time.Time,
) error {
	// 以防抖后的 next.Status 为准：抖动未达阈值时 next 仍为正常，不会建/销事件。
	if next.Status == core.MonitorStatusAbnormal {
		if prev.CurrentIncident != "" {
			return nil // 已有未结事件，沿用
		}
		inc := core.Incident{
			ID:            uuid.New().String(),
			EnvironmentID: env.ID,
			GroupID:       panel.GroupID,
			PanelID:       panel.ID,
			Status:        core.IncidentStatusOpen,
			Severity:      result.Severity,
			Title:         panel.Name + " 异常",
			Evidence:      result.Evidence,
			StartedAt:     now,
		}
		if err := a.monitorStore.SaveIncident(inc); err != nil {
			return fmt.Errorf("创建异常事件失败: %w", err)
		}
		next.CurrentIncident = inc.ID
		return nil
	}

	// 本轮恢复：若有未结事件则标记 resolved。
	if prev.CurrentIncident == "" {
		return nil
	}
	if inc, err := a.monitorStore.GetIncident(prev.CurrentIncident); err == nil {
		inc.Status = core.IncidentStatusResolved
		inc.ResolvedAt = &now
		if err := a.monitorStore.SaveIncident(inc); err != nil {
			return fmt.Errorf("关闭异常事件失败: %w", err)
		}
	}
	next.CurrentIncident = ""
	return nil
}

// RunPanelDiagnosis 对监控项手动触发一次 Troubleshoot Flow（异步）。
// 要求监控项当前有未结异常事件。本调用只把状态置为「诊断中」并立即返回，
// 真正的采集 + AI 分析 + 报告生成在后台进行——AI 较慢，异步可让概览实时显示「诊断中」、
// 避免阻塞前端调用。诊断完成后状态回到 abnormal 并挂上报告。
func (a *App) RunPanelDiagnosis(panelID string) error {
	if a.monitorStore == nil {
		return errors.New("监控存储未初始化")
	}
	panel, err := a.monitorStore.GetPanel(panelID)
	if err != nil {
		return err
	}
	state, ok, _ := a.monitorStore.GetPanelState(panelID)
	if !ok || state.CurrentIncident == "" {
		return errors.New("当前没有可诊断的异常事件")
	}
	inc, err := a.monitorStore.GetIncident(state.CurrentIncident)
	if err != nil {
		return err
	}
	env, err := a.environmentStore.Get(panel.EnvironmentID)
	if err != nil {
		return err
	}
	// 诊断去重：同一监控项只允许一个诊断在跑，避免重复触发 AI / 重复生成报告。
	if a.monitorDiagnosisGuard == nil || !a.monitorDiagnosisGuard.TryAcquire(panelID) {
		return errors.New("该监控项正在诊断中，请稍候")
	}

	// 同步置为「诊断中」，让概览立刻反映进度。
	inc.Status = core.IncidentStatusDiagnosing
	_ = a.monitorStore.SaveIncident(inc)
	state.Status = core.MonitorStatusDiagnosing
	_ = a.monitorStore.SavePanelState(state)

	go func() {
		defer a.monitorDiagnosisGuard.Release(panelID)
		if err := a.diagnosePanel(env, panel, inc); err != nil {
			zap.L().Warn("监控项诊断失败", zap.String("panel", panelID), zap.Error(err))
		}
	}()
	return nil
}

// diagnosePanel 在后台执行诊断：重新采集证据 → （可选）AI 分析 → 落报告 → 收尾状态。
func (a *App) diagnosePanel(env core.EnvironmentDef, panel core.MonitorPanel, inc core.Incident) error {
	registry, err := monitor.DefaultRegistry()
	if err != nil {
		return err
	}
	sources, err := a.monitorStore.ListSources(env.ID)
	if err != nil {
		return err
	}
	collector := monitor.NewCollector(registry)
	ctx, cancel := context.WithTimeout(context.Background(), monitorCollectionTimeout)
	defer cancel()
	batch := collector.RunCollection(ctx, env, sources, []core.MonitorPanel{panel})

	// AI 诊断器：复用 ops_doc 的 LLM 适配；未配置 API Key 时为 nil，报告降级为纯事实。
	summarize := monitor.Summarizer(a.makeReportSummarizer())
	tr := monitor.BuildTroubleshootReport(panel, inc, batch.Slice(panel.Requirements), summarize)

	now := time.Now()
	report := core.MonitorReport{
		ID:            uuid.New().String(),
		EnvironmentID: env.ID,
		IncidentID:    inc.ID,
		PanelID:       panel.ID,
		Title:         tr.Title,
		Summary:       tr.Summary,
		Content:       tr.Content,
		CreatedAt:     now,
	}
	if err := a.monitorStore.SaveReport(report); err != nil {
		return fmt.Errorf("保存诊断报告失败: %w", err)
	}

	// 事件回到 open（异常仍在）并挂报告。
	inc.Status = core.IncidentStatusOpen
	inc.ReportID = report.ID
	_ = a.monitorStore.SaveIncident(inc)

	// 收尾状态：以最新持久化状态为基础，解除「诊断中」回到 abnormal 并挂报告，
	// 避免覆盖诊断期间后台 tick 可能写入的其它字段。
	state, _, _ := a.monitorStore.GetPanelState(panel.ID)
	state.PanelID = panel.ID
	state.Status = core.MonitorStatusAbnormal
	state.LastReportID = report.ID
	state.HasHistory = true
	_ = a.monitorStore.SavePanelState(state)
	return nil
}

// AcknowledgePanelHistory 确认监控项异常已恢复并归档：把「正常（有异常历史）」收敛为纯正常。
// 仅在 history 状态有效；Incident 与报告记录仍保留（可在最近报告查看），只清除面板的待处理标记。
func (a *App) AcknowledgePanelHistory(panelID string) error {
	if a.monitorStore == nil {
		return errors.New("监控存储未初始化")
	}
	state, ok, err := a.monitorStore.GetPanelState(panelID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("监控项尚无状态")
	}
	if state.Status != core.MonitorStatusHistory {
		return errors.New("仅「正常（有异常历史）」可确认恢复")
	}
	state.Status = core.MonitorStatusNormal
	state.Severity = core.MonitorSeverityNone
	state.HasHistory = false
	state.Summary = "已确认恢复"
	if err := a.monitorStore.SavePanelState(state); err != nil {
		return fmt.Errorf("保存监控项状态失败: %w", err)
	}
	return nil
}

// ListIncidents 列出某环境的异常事件（最新在前）。
func (a *App) ListIncidents(environmentID string) ([]core.Incident, error) {
	if a.monitorStore == nil {
		return []core.Incident{}, nil
	}
	if strings.TrimSpace(environmentID) == "" {
		return nil, errors.New("environment_id 不能为空")
	}
	return a.monitorStore.ListIncidents(environmentID)
}

// GetIncident 按 ID 获取异常事件详情。
func (a *App) GetIncident(id string) (core.Incident, error) {
	if a.monitorStore == nil {
		return core.Incident{}, errors.New("监控存储未初始化")
	}
	return a.monitorStore.GetIncident(id)
}

// GetMonitorReport 按 ID 获取诊断报告。
func (a *App) GetMonitorReport(id string) (core.MonitorReport, error) {
	if a.monitorStore == nil {
		return core.MonitorReport{}, errors.New("监控存储未初始化")
	}
	return a.monitorStore.GetReport(id)
}

// assertEnvironmentExists 校验环境存在，作为监控实体挂载前的边界检查。
func (a *App) assertEnvironmentExists(environmentID string) error {
	if strings.TrimSpace(environmentID) == "" {
		return errors.New("environment_id 不能为空")
	}
	if a.environmentStore == nil {
		return errors.New("环境存储未初始化")
	}
	if _, err := a.environmentStore.Get(environmentID); err != nil {
		return fmt.Errorf("环境不存在: %s", environmentID)
	}
	return nil
}

// validateMonitorSourceKind 校验当前支持的监控源类型。
func validateMonitorSourceKind(kind string) error {
	switch kind {
	case core.MonitorSourceKindPrometheus:
		return nil
	case core.MonitorSourceKindBuiltin:
		return errors.New("内置监控源由系统自动提供，不能手动创建")
	default:
		return fmt.Errorf("不支持的监控源类型: %s", kind)
	}
}

// sanitizeMonitorSources 批量脱敏监控源配置。
func sanitizeMonitorSources(sources []core.MonitorSource) []core.MonitorSource {
	out := make([]core.MonitorSource, 0, len(sources))
	for _, source := range sources {
		out = append(out, sanitizeMonitorSource(source))
	}
	return out
}

// sanitizeMonitorSource 隐藏 token/password 等敏感字段，避免前端明文展示。
func sanitizeMonitorSource(source core.MonitorSource) core.MonitorSource {
	source.Config = normalizeSourceConfig(source.Config)
	for _, key := range []string{"password", "token"} {
		if strings.TrimSpace(fmt.Sprint(source.Config[key])) != "" {
			source.Config[key] = "******"
		}
	}
	return source
}

// normalizeSourceConfig 复制配置 map，避免直接修改调用方对象。
func normalizeSourceConfig(config map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range config {
		out[key] = value
	}
	return out
}

// mergeSourceSecretConfig 在编辑时保留未改动的敏感字段。
func mergeSourceSecretConfig(existing, next map[string]any) map[string]any {
	out := normalizeSourceConfig(next)
	for _, key := range []string{"password", "token"} {
		if strings.TrimSpace(fmt.Sprint(out[key])) == "******" {
			out[key] = existing[key]
		}
	}
	return out
}

// normalizeMonitorSourceID 为空时回落到内置源，兼容旧监控项。
func normalizeMonitorSourceID(sourceID string) string {
	if strings.TrimSpace(sourceID) == "" {
		return core.MonitorBuiltinSourceID
	}
	return sourceID
}
