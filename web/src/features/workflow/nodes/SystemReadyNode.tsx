// system_ready 节点：工作流启动时触发一次，无输入端口，一个 signal 输出
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { BaseNode } from './BaseNode';
import type { NodeInstance } from '@/types/workflow';

type SystemReadyNodeProps = NodeProps & { data: NodeInstance };

export function SystemReadyNode({ data, selected }: SystemReadyNodeProps) {
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
        id="signal"
        className="!h-3 !w-3 !bg-ready-600"
      />
      {/* data 被消费用于将来扩展（如显示节点 id 等） */}
      <span className="hidden">{data.instance_id}</span>
    </>
  );
}
