// assemble_start 节点：集合执行入口
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { BaseNode } from './BaseNode';
import type { NodeInstance } from '@/types/workflow';
import { getPortColor } from '@/types/nodeType';
import { useNodeExecState } from './useNodeExecState';

type Props = NodeProps & { data: NodeInstance };

export function AssembleStartNode({ data, selected }: Props) {
  const execState = useNodeExecState(data.instance_id);
  return (
    <BaseNode tone="ready" selected={selected} icon="▶️" title="Start" execState={execState}>
      <div style={{ height: 20 }} />
      <Handle
        type="source"
        position={Position.Right}
        id="exec_out"
        style={{
          top: 40,
          background: getPortColor('Exec'),
          width: 10,
          height: 10,
          borderRadius: 2,
          border: '2px solid rgba(0,0,0,0.15)',
        }}
        title="Exec Out"
      />
    </BaseNode>
  );
}
