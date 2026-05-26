// 节点类型定义，驱动前端节点展示、端口渲染与配置表单
// 与后端 core.NodeTypeDef / core.PortType 对齐

// ── 端口类型 ──────────────────────────────────────────────
// Exec = 执行流（UE 白色线），其余为数据流
// Dynamic = 变量节点等动态端口，真实类型从实例 config 中读取
export type PortType =
  | 'Exec'
  | 'String'
  | 'Int'
  | 'Float'
  | 'Bool'
  | 'Dynamic'
  | 'Any'
  | 'LinuxSshConnection'
  | 'LinuxFileHandle'
  | 'DockerContext'
  | 'K8sContext'
  | 'JenkinsContext'
  | 'NginxInstance';

// ── 节点分类 ──────────────────────────────────────────────
// 决定端口结构（有没有 exec 端口），与 UI 分组 category 不同
export type NodeKind = 'event' | 'action' | 'pure' | 'flow_control';

export type ExecutionMode = 'remote_cmd' | 'agent' | 'flow';

// 字段类型，对应后端 FieldSchema.Type
// variable_select：从当前图 variables 挑选，同步 var_type
// param_select / return_select：集合内从 params / returns 挑选，同步 var_type
// env_select：所有环境下拉；env_config_select：依赖同表单 environment_id + kind filter
export type FieldType =
  | 'text'
  | 'password'
  | 'number'
  | 'select'
  | 'toggle'
  | 'textarea'
  | 'variable_select'
  | 'param_select'
  | 'return_select'
  | 'env_select'
  | 'env_config_select';

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
  // env_config_select 用：限定环境内可选 config 的 kind
  config_kind_filter?: string;
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

// ── 系统事件源节点（仅出现在工作流中） ─────────────────────
export const SYSTEM_READY = 'system_ready';
export const SYSTEM_UPDATE = 'system_update';
export const SYSTEM_OVER = 'system_over';

// ── 集合内部节点 ──────────────────────────────────────────
export const ASSEMBLE_START = 'assemble_start';
export const ASSEMBLE_END = 'assemble_end';
export const ASSEMBLE_PARAM = 'assemble_param';

// 不可由用户在 AddNodeDialog 中手动添加、且不可删除的单例节点
const INTERNAL_NODE_TYPES = new Set([
  SYSTEM_READY,
  SYSTEM_UPDATE,
  SYSTEM_OVER,
  ASSEMBLE_START,
  ASSEMBLE_END,
]);

// 「绑定型」节点：config 必填一个绑定名（var_name / param_name / return_name），
// 否则保存时后端 validate*Refs 会拒收，AddNodeDialog 里裸建必失败。
// 这些节点全部走左侧栏拖入（自动带绑定）；可删除，但不能从 AddNodeDialog 创建。
const BINDING_NODE_TYPES = new Set([
  'var_get',
  'var_set',
  'assemble_param',
  'return_set',
]);

// 判断是否为系统/内部节点类型（不可手动添加、不可删除）
export function isInternalNodeType(typeId: string): boolean {
  return INTERNAL_NODE_TYPES.has(typeId);
}

// 判断是否为「绑定型」节点（不可从 AddNodeDialog 添加；走侧栏拖入）
export function isBindingNodeType(typeId: string): boolean {
  return BINDING_NODE_TYPES.has(typeId);
}

// 判断是否为工作流事件源节点
export function isSystemNodeType(typeId: string): boolean {
  return (
    typeId === SYSTEM_READY ||
    typeId === SYSTEM_UPDATE ||
    typeId === SYSTEM_OVER
  );
}

// 判断是否为集合内部节点（Start/End 单例；param/return getter/setter 走 generic）
export function isAssembleNodeType(typeId: string): boolean {
  return typeId === ASSEMBLE_START || typeId === ASSEMBLE_END;
}

// ── 端口颜色映射 ──────────────────────────────────────────
// 不同 PortType 用不同颜色，对齐 UE Blueprint 直觉
const PORT_COLORS: Record<string, string> = {
  Exec: '#e2e8f0',              // 白/浅灰（执行流）
  // 基础类型
  String: '#22c55e',            // 绿色
  Int: '#10b981',               // 翠绿
  Float: '#0ea5e9',             // 天蓝
  Bool: '#ef4444',              // 红色
  // 动态类型
  Dynamic: '#a855f7',           // 紫色
  Any: '#94a3b8',               // 灰（万能数据口）
  // 业务句柄
  LinuxSshConnection: '#3b82f6', // 蓝色
  LinuxFileHandle: '#eab308',   // 金黄（远程文件句柄）
  DockerContext: '#8b5cf6',     // 紫蓝
  DockerContainerHandle: '#7c3aed', // 深紫（容器引用，与 DockerContext 同色系）
  K8sContext: '#06b6d4',        // 青色
  JenkinsContext: '#d33833',    // 红 (Jenkins brand)
  NginxInstance: '#f97316',     // 橙色
};

// 获取端口颜色，未知类型回退灰色
export function getPortColor(portType: string): string {
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

// portTypesConnectable 判断数据端口类型是否可连线（Exec 必须同类；Any 可接任意数据类型）
export function portTypesConnectable(
  sourceType: PortType,
  targetType: PortType,
): boolean {
  if (sourceType === 'Exec' || targetType === 'Exec') {
    return sourceType === targetType;
  }
  if (sourceType === 'Any' || targetType === 'Any') {
    return true;
  }
  return sourceType === targetType;
}

// ── 新建节点时构造默认 config ─────────────────────────────
// 从 NodeTypeDef.config_schema 收集所有 default 字段，叠加用户覆盖
// 调用方：page 的 handleNodeTypeSelected、canvas 的 onDrop
export function buildDefaultConfig(
  typeDef: NodeTypeDef,
  overrides: Record<string, unknown> = {},
): Record<string, unknown> {
  const config: Record<string, unknown> = {};
  for (const field of typeDef.config_schema ?? []) {
    if (field.default !== undefined && field.default !== null) {
      config[field.id] = field.default;
    }
  }
  return { ...config, ...overrides };
}
