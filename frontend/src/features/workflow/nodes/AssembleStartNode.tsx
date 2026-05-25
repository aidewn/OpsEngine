// assemble_start 节点：集合执行入口
// 动态渲染 assemble.params → param_<name> 数据输出端口（与调用方 assemble 节点的入参对齐）

import { Handle, Position, type NodeProps } from '@xyflow/react';
import { BaseNode } from './BaseNode';
import type { NodeInstance } from '@/types/workflow';
import { getPortColor } from '@/types/nodeType';
import { useNodeExecState } from './useNodeExecState';
import { useCurrentAssemble } from '@/features/assemble/AssembleContext';

type Props = NodeProps & { data: NodeInstance };

const ROW_HEIGHT = 20;
const HEADER_OFFSET = 30;

export function AssembleStartNode({ data, selected }: Props) {
  const execState = useNodeExecState(data.instance_id);
  const assemble = useCurrentAssemble();
  const params = assemble?.params ?? [];

  return (
    <BaseNode
      tone="ready"
      selected={selected}
      icon="▶️"
      title="Start"
      execState={execState}
    >
      {/* exec_out 行 */}
      <div
        className="flex items-center justify-end text-[10px] text-slate-600"
        style={{ height: ROW_HEIGHT }}
      >
        <span className="pr-2.5">▶</span>
      </div>
      {/* 每个 param 一行：名称 + 类型 */}
      {params.map((p) => (
        <div
          key={p.name}
          className="flex items-center justify-end gap-2 text-[10px] text-slate-700"
          style={{ height: ROW_HEIGHT }}
        >
          <span className="truncate font-medium" title={`${p.name} (${p.var_type})`}>
            {p.name}
          </span>
          <span className="pr-2.5 text-slate-400">{p.var_type}</span>
        </div>
      ))}

      <Handle
        type="source"
        position={Position.Right}
        id="exec_out"
        style={{
          top: HEADER_OFFSET + ROW_HEIGHT / 2,
          background: getPortColor('Exec'),
          width: 12,
          height: 12,
          borderRadius: 2,
          border: '2px solid rgba(0,0,0,0.15)',
        }}
        title="Exec Out"
      />

      {params.map((p, idx) => (
        <Handle
          key={p.name}
          type="source"
          position={Position.Right}
          id={`param_${p.name}`}
          style={{
            top: HEADER_OFFSET + ROW_HEIGHT + idx * ROW_HEIGHT + ROW_HEIGHT / 2,
            background: getPortColor(p.var_type),
            width: 14,
            height: 14,
            borderRadius: 6,
            border: '2px solid rgba(0,0,0,0.15)',
          }}
          title={`${p.name} (${p.var_type})`}
        />
      ))}
    </BaseNode>
  );
}
