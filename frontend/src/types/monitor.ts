// 监控领域类型：与后端 internal/core/monitor.go 对齐。
// 时间字段在后端为 time.Time，经 Wails 序列化为 RFC3339 字符串。

// MonitorStatus 描述监控项在概览页中的展示状态。
export type MonitorStatus = 'normal' | 'abnormal' | 'diagnosing' | 'history';

// DataRequirement 描述监控项依赖的采集数据。
export interface DataRequirement {
  target_id: string;
  kind: string;
  params?: Record<string, unknown>;
}

// MonitorGroup 是环境下的监控分组。
export interface MonitorGroup {
  id: string;
  environment_id: string;
  name: string;
  description: string;
  enabled: boolean;
  order: number;
  created_at: string;
  updated_at: string;
}

// MonitorCondition 是监控项的一条阈值判断（Monitor Flow）。
export interface MonitorCondition {
  kind: string;
  target_id: string;
  field: string;
  op: '>' | '>=' | '<' | '<=' | '==' | '!=';
  value: number;
  severity: 'warning' | 'critical';
}

// MonitorPanel 是一个具体监控项。
export interface MonitorPanel {
  id: string;
  environment_id: string;
  group_id: string;
  name: string;
  description: string;
  enabled: boolean;
  requirements: DataRequirement[];
  conditions: MonitorCondition[];
  abnormal_threshold: number;
  recovery_threshold: number;
  monitor_flow_summary: string;
  troubleshoot_flow_summary: string;
  created_at: string;
  updated_at: string;
}

// PanelState 保存监控项当前状态和最近异常关联。
export interface PanelState {
  panel_id: string;
  status: MonitorStatus;
  severity: 'none' | 'warning' | 'critical';
  summary: string;
  last_checked_at?: string;
  current_incident_id?: string;
  last_report_id?: string;
  has_history: boolean;
}

// Incident 是监控异常事件。
export interface Incident {
  id: string;
  environment_id: string;
  group_id: string;
  panel_id: string;
  status: 'open' | 'diagnosing' | 'resolved';
  severity: 'warning' | 'critical';
  title: string;
  evidence: string[];
  trigger_value?: Record<string, unknown>;
  started_at: string;
  resolved_at?: string;
  report_id?: string;
}

// MonitorReport 是异常诊断完成后的报告摘要。
export interface MonitorReport {
  id: string;
  incident_id: string;
  panel_id: string;
  title: string;
  summary: string;
  content?: string;
  created_at: string;
}

// MonitorConfig 环境级监控调度配置（是否开启自动监控 + 采集间隔）。
export interface MonitorConfig {
  environment_id: string;
  enabled: boolean;
  interval_seconds: number;
  updated_at: string;
}

// MonitorOverviewData 聚合一个环境的监控首页所需数据。
export interface MonitorOverviewData {
  environment_id: string;
  groups: MonitorGroup[];
  panels: MonitorPanel[];
  states: PanelState[];
  incidents: Incident[];
  reports: MonitorReport[];
}
