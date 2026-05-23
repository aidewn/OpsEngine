// React Flow 的 nodeTypes 映射：type_id → 组件
// 新增节点组件时在这里登记；未登记的 type_id 走 GenericNode 兜底

import { type NodeTypes } from '@xyflow/react';
import { SystemReadyNode } from './SystemReadyNode';
import { SystemUpdateNode } from './SystemUpdateNode';
import { SystemOverNode } from './SystemOverNode';
import { GenericNode } from './GenericNode';
import { SYSTEM_OVER, SYSTEM_READY, SYSTEM_UPDATE } from '@/types/nodeType';

export const nodeTypeMap: NodeTypes = {
  [SYSTEM_READY]: SystemReadyNode,
  [SYSTEM_UPDATE]: SystemUpdateNode,
  [SYSTEM_OVER]: SystemOverNode,
  // React Flow 没有兜底机制，业务节点暂时全部 default 类型
  // 在 canvasMapping 中处理：未注册的 type_id 强制走 'generic'
  generic: GenericNode,
};
