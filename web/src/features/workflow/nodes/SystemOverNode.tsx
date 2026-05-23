// system_over 节点：工作流终止时触发一次
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { BaseNode } from './BaseNode';
import type { NodeInstance } from '@/types/workflow';

type Props = NodeProps & { data: NodeInstance };

export function SystemOverNode({ data, selected }: Props) {
  return (
    <>
      <BaseNode
        tone="over"
        selected={selected}
        category="Event · Over"
        title="System Over"
        subtitle="工作流终止时触发一次"
      />
      <Handle
        type="source"
        position={Position.Right}
        id="signal"
        className="!h-3 !w-3 !bg-over-600"
      />
      <span className="hidden">{data.instance_id}</span>
    </>
  );
}
