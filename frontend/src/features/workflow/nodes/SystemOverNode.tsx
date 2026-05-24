// system_over 节点：工作流终止时触发一次
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { BaseNode } from './BaseNode';
import type { NodeInstance } from '@/types/workflow';
import { getPortColor } from '@/types/nodeType';
import { useNodeExecState } from './useNodeExecState';

type Props = NodeProps & { data: NodeInstance };

export function SystemOverNode({ data, selected }: Props) {
  const execState = useNodeExecState(data.instance_id);
  return (
    <BaseNode
      tone="over"
      selected={selected}
      icon="🔴"
      title="System Over"
      execState={execState}
    >
      <div style={{ height: 20 }} />
      <Handle
        type="source"
        position={Position.Right}
        id="exec_out"
        style={{
          top: 40,
          background: getPortColor('Exec'),
          width: 12,
          height: 12,
          borderRadius: 2,
          border: '2px solid rgba(0,0,0,0.15)',
        }}
        title="Exec Out"
      />
    </BaseNode>
  );
}
