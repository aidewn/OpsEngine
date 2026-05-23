// 工作流相关类型，与后端 Go 结构对齐
// 字段命名遵循后端 JSON tag（snake_case）

export interface Position {
  x: number;
  y: number;
}

// 节点实例（工作流中的具体节点）
// 注意：不带 stage 字段，阶段归属由可达性导出
// 使用 type 而非 interface，满足 React Flow Node<T extends Record<string, unknown>> 约束
export type NodeInstance = {
  instance_id: string; // UUID
  type_id: string;
  config: Record<string, unknown>;
  state?: string;
  error_msg?: string;
  position: Position;
};

// 连线端口引用
export interface PortRef {
  node: string; // 引用 NodeInstance.instance_id
  port: string; // PortDef.id
}

export interface EdgeConfig {
  from: PortRef;
  to: PortRef;
}

// VariableDef 工作流/集合内部变量定义
export interface VariableDef {
  name: string;
  var_type: string; // 复用 PortType 字符串
  default?: unknown;
}

// 工作流定义（顶层结构，与后端 core.WorkflowDef 对齐）
export interface WorkflowDef {
  id: string;
  name: string;
  description: string;
  variables: VariableDef[];
  nodes: NodeInstance[];
  edges: EdgeConfig[];
}
