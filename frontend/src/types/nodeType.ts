// 节点类型定义，驱动前端节点展示、端口渲染与配置表单
// 与后端 core.NodeTypeDef / core.PortType 对齐

// ── 端口类型 ──────────────────────────────────────────────
// Exec = 执行流（UE 白色线），其余为数据流
// Dynamic = 变量节点等动态端口，真实类型从实例 config 中读取
export type PortType =
  | 'Exec'
  | 'String'
  | 'Dynamic'
  | 'LinuxSshConnection'
  | 'DockerContext'
  | 'K8sContext'
  | 'NginxInstance';

// ── 节点分类 ──────────────────────────────────────────────
// 决定端口结构（有没有 exec 端口），与 UI 分组 category 不同
export type NodeKind = 'event' | 'action' | 'pure' | 'flow_control';

export type ExecutionMode = 'remote_cmd' | 'agent' | 'flow';

// 字段类型，对应后端 FieldSchema.Type
export type FieldType = 'text' | 'password' | 'number' | 'select' | 'toggle';

export interface PortDef {
  id: string;
  label: string;
  port_type: PortType;
  required: boolean;
}

export interface FieldSchema {
  type: FieldType;
  id: string;
  label: string;
  placeholder?: string;
  required?: boolean;
  min?: number;
  max?: number;
  default?: unknown;
  options?: string[];
}

export interface NodeTypeDef {
  type_id: string;
  display_name: string;
  category: string;
  node_kind: NodeKind;
  icon: string;
  description: string;
  input_ports: PortDef[];
  output_ports: PortDef[];
  config_schema: FieldSchema[];
  execution_mode: ExecutionMode;
}

// ── 系统事件源节点 ────────────────────────────────────────
export const SYSTEM_READY = 'system_ready';
export const SYSTEM_UPDATE = 'system_update';
export const SYSTEM_OVER = 'system_over';

export type SystemNodeType =
  | typeof SYSTEM_READY
  | typeof SYSTEM_UPDATE
  | typeof SYSTEM_OVER;

export function isSystemNodeType(typeId: string): typeId is SystemNodeType {
  return (
    typeId === SYSTEM_READY ||
    typeId === SYSTEM_UPDATE ||
    typeId === SYSTEM_OVER
  );
}

// ── 端口颜色映射 ──────────────────────────────────────────
// 不同 PortType 用不同颜色，对齐 UE Blueprint 直觉
const PORT_COLORS: Record<PortType, string> = {
  Exec: '#e2e8f0',              // 白/浅灰（执行流）
  String: '#22c55e',            // 绿色
  Dynamic: '#a855f7',           // 紫色（动态类型）
  LinuxSshConnection: '#3b82f6', // 蓝色
  DockerContext: '#8b5cf6',     // 紫蓝
  K8sContext: '#06b6d4',        // 青色
  NginxInstance: '#f97316',     // 橙色
};

// 获取端口颜色，未知类型回退灰色
export function getPortColor(portType: PortType): string {
  return PORT_COLORS[portType] ?? '#94a3b8';
}

// ── 动态端口类型解析 ──────────────────────────────────────
// 变量节点的 output port_type 为 Dynamic，真实类型从 config.var_type 读取
export function resolvePortType(
  portDef: PortDef,
  config: Record<string, unknown>,
): PortType {
  if (portDef.port_type === 'Dynamic') {
    const varType = config['var_type'];
    if (typeof varType === 'string' && varType in PORT_COLORS) {
      return varType as PortType;
    }
    return 'String'; // 默认 fallback
  }
  return portDef.port_type;
}
