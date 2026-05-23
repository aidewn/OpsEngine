// 通用业务节点：根据 NodeTypeDef 动态渲染所有端口
// 系统节点以外的所有节点都由此组件渲染

import { Handle, Position, type NodeProps } from '@xyflow/react';
import { BaseNode } from './BaseNode';
import type { NodeInstance } from '@/types/workflow';
import { useNodeTypes } from '@/api/nodeTypes';
import { getPortColor, resolvePortType } from '@/types/nodeType';

type Props = NodeProps & { data: NodeInstance };

// 端口纵向间距（px）
const PORT_SPACING = 22;
// 第一个端口的起始 top 偏移
const PORT_START = 42;

export function GenericNode({ data, selected }: Props) {
  const { data: nodeTypes } = useNodeTypes();
  const def = nodeTypes?.find((t) => t.type_id === data.type_id);

  // 分离 exec 端口和数据端口，exec 端口排在最上方
  const inputPorts = def?.input_ports ?? [];
  const outputPorts = def?.output_ports ?? [];

  return (
    <>
      {/* 左侧输入端口 */}
      {inputPorts.map((port, idx) => {
        const realType = resolvePortType(port, data.config);
        const color = getPortColor(realType);
        const isExec = realType === 'Exec';
        return (
          <Handle
            key={port.id}
            type="target"
            position={Position.Left}
            id={port.id}
            style={{
              top: PORT_START + idx * PORT_SPACING,
              background: color,
              width: isExec ? 10 : 12,
              height: isExec ? 10 : 12,
              borderRadius: isExec ? 2 : 6,
              border: '2px solid rgba(0,0,0,0.15)',
            }}
            title={`${port.label} (${realType})`}
          />
        );
      })}

      <BaseNode
        tone="neutral"
        selected={selected}
        category={def?.category ?? data.type_id}
        title={def?.display_name ?? data.type_id}
        subtitle={def?.description}
      />

      {/* 右侧输出端口 */}
      {outputPorts.map((port, idx) => {
        const realType = resolvePortType(port, data.config);
        const color = getPortColor(realType);
        const isExec = realType === 'Exec';
        return (
          <Handle
            key={port.id}
            type="source"
            position={Position.Right}
            id={port.id}
            style={{
              top: PORT_START + idx * PORT_SPACING,
              background: color,
              width: isExec ? 10 : 12,
              height: isExec ? 10 : 12,
              borderRadius: isExec ? 2 : 6,
              border: '2px solid rgba(0,0,0,0.15)',
            }}
            title={`${port.label} (${realType})`}
          />
        );
      })}
    </>
  );
}
