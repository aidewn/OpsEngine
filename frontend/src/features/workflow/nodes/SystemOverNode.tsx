// system_over 节点：工作流终止时触发一次
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { BaseNode } from './BaseNode';
import type { NodeInstance } from '@/types/workflow';
import { getPortColor } from '@/types/nodeType';

type Props = NodeProps & { data: NodeInstance };

export function SystemOverNode({ data, selected }: Props) {
  void data;
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
        id="exec_out"
        style={{
          background: getPortColor('Exec'),
          width: 10,
          height: 10,
          borderRadius: 2,
          border: '2px solid rgba(0,0,0,0.15)',
        }}
        title="Exec Out"
      />
    </>
  );
}
