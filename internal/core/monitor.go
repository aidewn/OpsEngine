package core

import "time"

// MonitorSourceKindBuiltin 是环境内置采集源，桥接现有 SSH/Docker/K8s/HTTP 能力。
const MonitorSourceKindBuiltin = "builtin"

// MonitorSourceKindPrometheus 是 Prometheus HTTP API 监控源。
const MonitorSourceKindPrometheus = "prometheus"

// MonitorBuiltinSourceID 是旧监控项和内置采集源使用的默认 source_id。
const MonitorBuiltinSourceID = "builtin"

// 监控领域模型（Phase 1：信息架构与静态管理）
//
// 设计取舍（对齐 docs/monitoring-architecture-plan.md）：
//   - 监控始终依赖环境，所有实体都带 EnvironmentID，不跨环境混合；
//   - 监控项（MonitorPanel）是最小监控单元，对应一个用户关心的问题；
//   - Phase 1 只落盘分组与监控项；PanelState / Incident / MonitorReport 的生成
//     属于 Phase 3/4（Monitor Flow 与 Incident），此处仅先定义形态供概览聚合使用。
//   - 监控项的 PanelFlow / TriggerPolicy 等运行策略留到 Phase 2+，Phase 1 先用
//     两段流程摘要字符串占位，与前端骨架契约保持一致。

// MonitorStatus 监控项在概览页的展示状态（四态收敛，见 plan §3.2）
type MonitorStatus string

const (
	MonitorStatusNormal     MonitorStatus = "normal"     // 正常：当前无异常且无未处理异常历史
	MonitorStatusAbnormal   MonitorStatus = "abnormal"   // 有异常：已发现异常，尚未完成诊断
	MonitorStatusDiagnosing MonitorStatus = "diagnosing" // 诊断中：排查流程正在执行
	MonitorStatusHistory    MonitorStatus = "history"    // 正常（有异常历史）：已恢复但有最近报告
)

// MonitorSeverity 异常严重级别
type MonitorSeverity string

const (
	MonitorSeverityNone     MonitorSeverity = "none"
	MonitorSeverityWarning  MonitorSeverity = "warning"
	MonitorSeverityCritical MonitorSeverity = "critical"
)

// IncidentStatus 异常事件状态
type IncidentStatus string

const (
	IncidentStatusOpen       IncidentStatus = "open"
	IncidentStatusDiagnosing IncidentStatus = "diagnosing"
	IncidentStatusResolved   IncidentStatus = "resolved"
)

// MonitorSource 是环境级监控源配置。
// 内置源可以虚拟合成；外部源（如 Prometheus）落盘保存。
type MonitorSource struct {
	ID            string         `json:"id"             toml:"id"`
	EnvironmentID string         `json:"environment_id" toml:"environment_id"`
	Name          string         `json:"name"           toml:"name"`
	Kind          string         `json:"kind"           toml:"kind"`
	Enabled       bool           `json:"enabled"        toml:"enabled"`
	Config        map[string]any `json:"config"         toml:"config"`
	CreatedAt     time.Time      `json:"created_at"     toml:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"     toml:"updated_at"`
}

// DataRequirement 描述监控项依赖的一项采集数据（plan §8.3）
// SourceID 引用监控源；TargetID 在 builtin 源下引用环境内 SSH/Docker/K8s 等配置。
type DataRequirement struct {
	SourceID string         `json:"source_id"           toml:"source_id"`
	TargetID string         `json:"target_id,omitempty" toml:"target_id,omitempty"`
	Kind     string         `json:"kind"                toml:"kind"`
	Params   map[string]any `json:"params"              toml:"params"`
}

// MonitorGroup 环境下的监控分组（plan §8.1）
// 分组本身不采集数据，只负责组织监控项与展示聚合状态。
type MonitorGroup struct {
	ID            string    `json:"id"             toml:"id"`
	EnvironmentID string    `json:"environment_id" toml:"environment_id"`
	Name          string    `json:"name"           toml:"name"`
	Description   string    `json:"description"    toml:"description"`
	Order         int       `json:"order"          toml:"order"`
	Enabled       bool      `json:"enabled"        toml:"enabled"`
	CreatedAt     time.Time `json:"created_at"     toml:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"     toml:"updated_at"`
}

// MonitorPanel 一个具体监控项（plan §8.2）
// 对应一个用户关心的问题（如「CPU 使用率是否大于 80%」），而不是一条原始指标。
type MonitorPanel struct {
	ID            string            `json:"id"             toml:"id"`
	EnvironmentID string            `json:"environment_id" toml:"environment_id"`
	GroupID       string            `json:"group_id"       toml:"group_id"`
	Name          string            `json:"name"           toml:"name"`
	Description   string            `json:"description"    toml:"description"`
	Enabled       bool              `json:"enabled"        toml:"enabled"`
	Requirements  []DataRequirement `json:"requirements"   toml:"requirements"`
	// Conditions 是轻量 Monitor Flow（Phase 3）：基于本轮采集结果做阈值判断。
	// 任一条件命中即视为异常；severity 取命中条件中的最高级别。
	Conditions []MonitorCondition `json:"conditions" toml:"conditions"`
	// 防抖阈值（每监控项独立）：连续异常≥AbnormalThreshold 才告警、连续正常≥RecoveryThreshold 才解除。
	// 1 表示即时翻转（不防抖）；不同指标抖动特性不同，故配在监控项上。
	AbnormalThreshold int `json:"abnormal_threshold" toml:"abnormal_threshold"`
	RecoveryThreshold int `json:"recovery_threshold" toml:"recovery_threshold"`
	// MonitorFlowSummary / TroubleshootFlowSummary 为占位描述；
	// 可执行的排查流程（Troubleshoot Flow）留到 Phase 4+。
	MonitorFlowSummary      string    `json:"monitor_flow_summary"      toml:"monitor_flow_summary"`
	TroubleshootFlowSummary string    `json:"troubleshoot_flow_summary" toml:"troubleshoot_flow_summary"`
	CreatedAt               time.Time `json:"created_at"                toml:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"                toml:"updated_at"`
}

// MonitorCondition 是 Monitor Flow 的单条阈值判断（plan §6.1）。
// 从某类采集结果（Kind，可选指定 TargetID）中按 Field 取数值，与 Value 按 Op 比较。
// Field 支持单层数组展开，如 "filesystems[].use_percent"——任一元素命中即算命中。
type MonitorCondition struct {
	SourceID string          `json:"source_id"           toml:"source_id"`
	Kind     string          `json:"kind"                toml:"kind"`
	TargetID string          `json:"target_id,omitempty" toml:"target_id,omitempty"`
	Field    string          `json:"field"               toml:"field"`
	Op       string          `json:"op"                  toml:"op"` // > >= < <= == !=
	Value    float64         `json:"value"               toml:"value"`
	Severity MonitorSeverity `json:"severity"            toml:"severity"`
}

// PanelState 监控项当前状态与最近异常关联（plan §8.4）
// Phase 3 起由 Monitor Flow 更新并持久化。
type PanelState struct {
	PanelID         string          `json:"panel_id"                     toml:"panel_id"`
	Status          MonitorStatus   `json:"status"                       toml:"status"`
	Severity        MonitorSeverity `json:"severity"                     toml:"severity"`
	Summary         string          `json:"summary"                      toml:"summary"`
	LastCheckedAt   *time.Time      `json:"last_checked_at,omitempty"    toml:"last_checked_at,omitempty"`
	CurrentIncident string          `json:"current_incident_id,omitempty" toml:"current_incident_id,omitempty"`
	LastReportID    string          `json:"last_report_id,omitempty"     toml:"last_report_id,omitempty"`
	HasHistory      bool            `json:"has_history"                  toml:"has_history"`
	// 连续计数用于防抖：异常需连续达阈值才告警、恢复需连续达阈值才解除（plan §5.3）。
	ConsecutiveAbnormal int `json:"consecutive_abnormal" toml:"consecutive_abnormal"`
	ConsecutiveNormal   int `json:"consecutive_normal"   toml:"consecutive_normal"`
}

// Incident 监控异常事件（plan §8.5）。Phase 4 起生成。
type Incident struct {
	ID            string          `json:"id"             toml:"id"`
	EnvironmentID string          `json:"environment_id" toml:"environment_id"`
	GroupID       string          `json:"group_id"       toml:"group_id"`
	PanelID       string          `json:"panel_id"       toml:"panel_id"`
	Status        IncidentStatus  `json:"status"         toml:"status"`
	Severity      MonitorSeverity `json:"severity"       toml:"severity"`
	Title         string          `json:"title"          toml:"title"`
	Evidence      []string        `json:"evidence"       toml:"evidence"`
	StartedAt     time.Time       `json:"started_at"     toml:"started_at"`
	ResolvedAt    *time.Time      `json:"resolved_at,omitempty" toml:"resolved_at,omitempty"`
	ReportID      string          `json:"report_id,omitempty"   toml:"report_id,omitempty"`
}

// MonitorReport 异常诊断报告（plan §8.6）。Phase 4 起生成。
type MonitorReport struct {
	ID            string    `json:"id"             toml:"id"`
	EnvironmentID string    `json:"environment_id" toml:"environment_id"`
	IncidentID    string    `json:"incident_id"    toml:"incident_id"`
	PanelID       string    `json:"panel_id"       toml:"panel_id"`
	Title         string    `json:"title"          toml:"title"`
	Summary       string    `json:"summary"        toml:"summary"`
	Content       string    `json:"content,omitempty" toml:"content,omitempty"`
	CreatedAt     time.Time `json:"created_at"     toml:"created_at"`
}

// MonitorConfig 环境级监控调度配置：是否开启自动监控及采集间隔。
// 后台调度器按各环境的间隔周期性驱动一轮 tick（plan §4.3）。
type MonitorConfig struct {
	EnvironmentID   string    `json:"environment_id"   toml:"environment_id"`
	Enabled         bool      `json:"enabled"          toml:"enabled"`
	IntervalSeconds int       `json:"interval_seconds" toml:"interval_seconds"`
	UpdatedAt       time.Time `json:"updated_at"        toml:"updated_at"`
}

// MonitorOverview 聚合一个环境监控首页所需数据（plan §3）
type MonitorOverview struct {
	EnvironmentID string          `json:"environment_id"`
	Sources       []MonitorSource `json:"sources"`
	Groups        []MonitorGroup  `json:"groups"`
	Panels        []MonitorPanel  `json:"panels"`
	States        []PanelState    `json:"states"`
	Incidents     []Incident      `json:"incidents"`
	Reports       []MonitorReport `json:"reports"`
}
