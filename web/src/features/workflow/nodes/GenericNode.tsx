// 兜底节点：未注册到 nodeTypeMap 的业务节点都用这个组件渲染
// MVP 阶段只渲染 3 个系统节点，业务节点暂留这个兜底实现
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { BaseNode } from './BaseNode';
import type { NodeInstance } from '@/types/workflow';
import { useNodeTypes } from '@/api/nodeTypes';

type Props = NodeProps & { data: NodeInstance };

export function GenericNode({ data, selected }: Props) {
  const { data: nodeTypes } = useNodeTypes();
  const def = nodeTypes?.find((t) => t.type_id === data.type_id);

  return (
    <>
      <Handle
        type="target"
        position={Position.Left}
        id="trigger"
        className="!h-3 !w-3 !bg-slate-500"
      />
      <BaseNode
        tone="neutral"
        selected={selected}
        category={def?.category ?? data.type_id}
        title={def?.display_name ?? data.type_id}
        subtitle={def?.description}
      />
      {/* 业务节点的输出端口（按 NodeTypeDef 渲染） */}
      {def?.output_ports.map((port, idx) => (
        <Handle
          key={port.id}
          type="source"
          position={Position.Right}
          id={port.id}
          style={{ top: 40 + idx * 18 }}
          className="!h-3 !w-3 !bg-slate-500"
        />
      ))}
    </>
  );
}
