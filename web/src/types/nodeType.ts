// 节点类型定义（GET /api/node-types 返回），驱动前端节点展示与配置表单

export type PortType =
  | 'LinuxSshConnection'
  | 'DockerContext'
  | 'K8sContext'
  | 'NginxInstance'
  | 'FlowSignal';

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
  icon: string;
  description: string;
  input_ports: PortDef[];
  output_ports: PortDef[];
  config_schema: FieldSchema[];
  execution_mode: ExecutionMode;
}

// 系统事件源节点 TypeID 常量
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
