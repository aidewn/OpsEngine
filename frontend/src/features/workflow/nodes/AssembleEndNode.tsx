// assemble_end 节点：集合执行出口
// input 端口 = exec_in + 每个 return 一个数据端口（动态根据 AssembleContext.returns 渲染）

import { Handle, Position, type NodeProps } from '@xyflow/react';
import { BaseNode } from './BaseNode';
import type { NodeInstance } from '@/types/workflow';
import { getPortColor } from '@/types/nodeType';
import { useNodeExecState } from './useNodeExecState';
import { useCurrentAssemble } from '@/features/assemble/AssembleContext';

type Props = NodeProps & { data: NodeInstance };

const ROW_HEIGHT = 20;
const HEADER_OFFSET = 30;

export function AssembleEndNode({ data, selected }: Props) {
  const execState = useNodeExecState(data.instance_id);
  const assemble = useCurrentAssemble();
  const returns = assemble?.returns ?? [];

  return (
    <BaseNode
      tone="over"
      selected={selected}
      icon="⏹️"
      title="End"
      execState={execState}
    >
      {/* exec_in 行 */}
      <div
        className="flex items-center text-[10px] text-slate-600"
        style={{ height: ROW_HEIGHT }}
      />
      {/* 每个 return 一行 */}
      {returns.map((r) => (
        <div
          key={r.name}
          className="flex items-center text-[10px] text-slate-700"
          style={{ height: ROW_HEIGHT }}
        >
          <span className="pl-2.5 font-medium">{r.var_type}</span>
        </div>
      ))}

      {/* exec_in handle */}
      <Handle
        type="target"
        position={Position.Left}
        id="exec_in"
        style={{
          top: HEADER_OFFSET + ROW_HEIGHT / 2,
          background: getPortColor('Exec'),
          width: 10,
          height: 10,
          borderRadius: 2,
          border: '2px solid rgba(0,0,0,0.15)',
        }}
        title="Exec In"
      />

      {/* 每个 return 的 input handle */}
      {returns.map((r, idx) => (
        <Handle
          key={r.name}
          type="target"
          position={Position.Left}
          id={`return_${r.name}`}
          style={{
            top: HEADER_OFFSET + ROW_HEIGHT + idx * ROW_HEIGHT + ROW_HEIGHT / 2,
            background: getPortColor(r.var_type),
            width: 11,
            height: 11,
            borderRadius: 6,
            border: '2px solid rgba(0,0,0,0.15)',
          }}
          title={`${r.name} (${r.var_type})`}
        />
      ))}
    </BaseNode>
  );
}
