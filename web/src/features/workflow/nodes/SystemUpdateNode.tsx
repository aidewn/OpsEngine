// system_update 节点：按 delta 周期循环触发
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { BaseNode } from './BaseNode';
import type { NodeInstance } from '@/types/workflow';

type Props = NodeProps & { data: NodeInstance };

export function SystemUpdateNode({ data, selected }: Props) {
  const deltaType = (data.config?.delta_type as string) ?? 'manual';
  const deltaSeconds = data.config?.delta_seconds as number | undefined;

  const subtitle =
    deltaType === 'interval' && deltaSeconds
      ? `每 ${deltaSeconds} 秒触发`
      : deltaType === 'cron'
        ? `cron: ${(data.config?.cron_expr as string) ?? '(未配置)'}`
        : '手动触发（只执行一次）';

  return (
    <>
      <BaseNode
        tone="update"
        selected={selected}
        category="Event · Update"
        title="System Update"
        subtitle={subtitle}
      />
      <Handle
        type="source"
        position={Position.Right}
        id="signal"
        className="!h-3 !w-3 !bg-update-600"
      />
    </>
  );
}
