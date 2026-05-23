// system_ready 节点：工作流启动时触发一次，无输入端口，一个 exec 输出
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { BaseNode } from './BaseNode';
import type { NodeInstance } from '@/types/workflow';
import { getPortColor } from '@/types/nodeType';

type Props = NodeProps & { data: NodeInstance };

export function SystemReadyNode({ data, selected }: Props) {
  void data;
  return (
    <>
      <BaseNode
        tone="ready"
        selected={selected}
        category="Event · Ready"
        title="System Ready"
        subtitle="工作流启动时触发一次"
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
