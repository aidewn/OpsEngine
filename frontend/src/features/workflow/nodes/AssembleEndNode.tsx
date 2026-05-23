// assemble_end 节点：集合执行出口
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { BaseNode } from './BaseNode';
import type { NodeInstance } from '@/types/workflow';
import { getPortColor } from '@/types/nodeType';

type Props = NodeProps & { data: NodeInstance };

export function AssembleEndNode({ selected }: Props) {
  return (
    <BaseNode tone="over" selected={selected} icon="⏹️" title="End">
      <div style={{ height: 20 }} />
      <Handle
        type="target"
        position={Position.Left}
        id="exec_in"
        style={{
          top: 40,
          background: getPortColor('Exec'),
          width: 10,
          height: 10,
          borderRadius: 2,
          border: '2px solid rgba(0,0,0,0.15)',
        }}
        title="Exec In"
      />
    </BaseNode>
  );
}
