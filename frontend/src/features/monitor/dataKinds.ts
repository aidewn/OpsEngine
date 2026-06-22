// 监控数据需求的类型与参数定义，与后端 internal/monitor 的 DataSource 一一对应。
// 用于驱动监控项「数据需求」编辑器：按 kind 渲染目标选择与参数表单。
import type { EnvConfigKind } from '@/types/environment';

// ParamField 描述某个 kind 下的一个参数输入。
export interface ParamField {
  key: string;
  label: string;
  type: 'text' | 'number' | 'toggle';
  placeholder?: string;
}

// FieldOption 是该 kind 下可用于判断条件的数值字段。
export interface FieldOption {
  value: string;
  label: string;
}

// MonitorKindDef 描述一类可采集数据。
// configKind 为空表示该类型不依赖环境配置（如 http.health 直接打 URL）。
// conditionFields 列出该类型可做阈值判断的数值字段路径（支持 [] 数组展开）。
export interface MonitorKindDef {
  kind: string;
  label: string;
  sourceKind: 'builtin' | 'prometheus';
  configKind?: EnvConfigKind;
  params: ParamField[];
  conditionFields?: FieldOption[];
}

// MONITOR_KINDS 是前端可选的数据类型清单（与后端注册的 DataSource 对齐）。
export const MONITOR_KINDS: MonitorKindDef[] = [
  {
    kind: 'host.basic',
    label: '主机基础指标（CPU/内存/负载）',
    sourceKind: 'builtin',
    configKind: 'ssh',
    params: [],
    conditionFields: [
      { value: 'cpu_usage_percent', label: 'CPU 使用率(%)' },
      { value: 'mem_usage_percent', label: '内存使用率(%)' },
      { value: 'load1', label: '1 分钟负载' },
      { value: 'load5', label: '5 分钟负载' },
      { value: 'load15', label: '15 分钟负载' },
    ],
  },
  {
    kind: 'host.disk',
    label: '磁盘水位',
    sourceKind: 'builtin',
    configKind: 'ssh',
    params: [{ key: 'mount', label: '挂载点（可选）', type: 'text', placeholder: '/' }],
    conditionFields: [{ value: 'filesystems[].use_percent', label: '任一分区使用率(%)' }],
  },
  {
    kind: 'docker.containers',
    label: 'Docker 容器',
    sourceKind: 'builtin',
    configKind: 'docker',
    params: [
      { key: 'filter_name', label: '名称过滤（可选）', type: 'text' },
      { key: 'all', label: '包含已停止容器', type: 'toggle' },
    ],
  },
  {
    kind: 'k8s.workloads',
    label: 'K8s 工作负载',
    sourceKind: 'builtin',
    configKind: 'k8s',
    params: [{ key: 'namespace', label: '命名空间（可选）', type: 'text' }],
    conditionFields: [
      { value: 'workloads[].unavailable_replicas', label: '任一负载不可用副本数' },
    ],
  },
  {
    kind: 'http.health',
    label: 'HTTP 健康检查',
    sourceKind: 'builtin',
    params: [
      { key: 'url', label: 'URL', type: 'text', placeholder: 'https://example.com/health' },
      { key: 'expect_status', label: '期望状态码（可选）', type: 'number' },
    ],
    conditionFields: [
      { value: 'status_code', label: 'HTTP 状态码' },
      { value: 'latency_ms', label: '响应时间(ms)' },
    ],
  },
  {
    kind: 'prometheus.query',
    label: 'Prometheus 即时查询',
    sourceKind: 'prometheus',
    params: [{ key: 'query', label: 'PromQL', type: 'text', placeholder: 'up' }],
    conditionFields: [{ value: 'value', label: '查询结果值' }],
  },
];

// kindDef 按 kind 查定义。
export function kindDef(kind: string): MonitorKindDef | undefined {
  return MONITOR_KINDS.find((k) => k.kind === kind);
}
