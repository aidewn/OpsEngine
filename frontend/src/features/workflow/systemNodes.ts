// 创建工作流时前端自动放置 3 个事件源节点
// 这是纯前端便利功能，后端不感知

import type { NodeInstance } from '@/types/workflow';
import { SYSTEM_OVER, SYSTEM_READY, SYSTEM_UPDATE } from '@/types/nodeType';
import { newUUID } from '@/lib/uuid';

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
