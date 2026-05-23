// 集合相关类型，与后端 core.AssembleDef 对齐

import type { EdgeConfig, NodeInstance, VariableDef } from './workflow';

// ParamDef 集合的参数/返回值定义
export interface ParamDef {
  name: string;
  var_type: string; // 复用 PortType 字符串
}

// 集合定义（用户创建的可复用节点图，不能单独运行）
export interface AssembleDef {
  id: string;
  name: string;
  description: string;
  params: ParamDef[];
  returns: ParamDef[];
  variables: VariableDef[];
  nodes: NodeInstance[];
  edges: EdgeConfig[];
}
