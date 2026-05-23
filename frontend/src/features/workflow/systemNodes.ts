// 创建工作流/集合时前端自动放置默认节点
// 这是纯前端便利功能，后端不感知

import type { NodeInstance } from '@/types/workflow';
import {
  SYSTEM_READY,
  SYSTEM_UPDATE,
  SYSTEM_OVER,
  ASSEMBLE_START,
  ASSEMBLE_END,
} from '@/types/nodeType';
import { newUUID } from '@/lib/uuid';

// 工作流默认节点：3 个事件源
export function createDefaultSystemNodes(): NodeInstance[] {
  return [
    {
      instance_id: newUUID(),
      type_id: SYSTEM_READY,
      config: {},
      position: { x: 80, y: 80 },
    },
    {
      instance_id: newUUID(),
      type_id: SYSTEM_UPDATE,
      config: { delta_type: 'interval', delta_seconds: 60 },
      position: { x: 80, y: 280 },
    },
    {
      instance_id: newUUID(),
      type_id: SYSTEM_OVER,
      config: {},
      position: { x: 80, y: 480 },
    },
  ];
}

// 集合默认节点：Start + End
export function createDefaultAssembleNodes(): NodeInstance[] {
  return [
    {
      instance_id: newUUID(),
      type_id: ASSEMBLE_START,
      config: {},
      position: { x: 80, y: 200 },
    },
    {
      instance_id: newUUID(),
      type_id: ASSEMBLE_END,
      config: {},
      position: { x: 600, y: 200 },
    },
  ];
}
