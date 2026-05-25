// React Flow 的 nodeTypes 映射：type_id → 组件
// 新增节点组件时在这里登记；未登记的 type_id 走 GenericNode 兜底

import { type NodeTypes } from '@xyflow/react';
import { SystemReadyNode } from './SystemReadyNode';
import { SystemUpdateNode } from './SystemUpdateNode';
import { SystemOverNode } from './SystemOverNode';
import { AssembleStartNode } from './AssembleStartNode';
import { AssembleEndNode } from './AssembleEndNode';
import { GenericNode } from './GenericNode';
import {
  SYSTEM_READY,
  SYSTEM_UPDATE,
  SYSTEM_OVER,
  ASSEMBLE_START,
  ASSEMBLE_END,
  ASSEMBLE_PARAM,
} from '@/types/nodeType';

export const nodeTypeMap: NodeTypes = {
  // 工作流事件源
  [SYSTEM_READY]: SystemReadyNode,
  [SYSTEM_UPDATE]: SystemUpdateNode,
  [SYSTEM_OVER]: SystemOverNode,
  // 集合内部节点
  [ASSEMBLE_START]: AssembleStartNode,
  [ASSEMBLE_END]: AssembleEndNode,
  // assemble_param：Dynamic 输出端口，复用 GenericNode 渲染
  [ASSEMBLE_PARAM]: GenericNode,
  // 其他业务节点统一走 generic
  generic: GenericNode,
};
