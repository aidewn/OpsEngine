// assemble_end 节点：集合执行出口
// 动态渲染 assemble.returns → return_<name> 数据输入端口；到达 End 时引擎收集到 frame.Returns

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
      >
        <span className="pl-2.5">▶</span>
      </div>
      {/* 每个 return 一行 */}
      {returns.map((r) => (
        <div
          key={r.name}
          className="flex items-center gap-2 text-[10px] text-slate-700"
          style={{ height: ROW_HEIGHT }}
        >
          <span className="pl-2.5 font-medium">{r.name}</span>
          <span className="text-slate-400">{r.var_type}</span>
        </div>
      ))}

      <Handle
        type="target"
        position={Position.Left}
        id="exec_in"
        style={{
          top: HEADER_OFFSET + ROW_HEIGHT / 2,
          background: getPortColor('Exec'),
          width: 12,
          height: 12,
          borderRadius: 2,
          border: '2px solid rgba(0,0,0,0.15)',
        }}
        title="Exec In"
      />

      {returns.map((r, idx) => (
        <Handle
          key={r.name}
          type="target"
          position={Position.Left}
          id={`return_${r.name}`}
          style={{
            top: HEADER_OFFSET + ROW_HEIGHT + idx * ROW_HEIGHT + ROW_HEIGHT / 2,
            background: getPortColor(r.var_type),
            width: 14,
            height: 14,
            borderRadius: 6,
            border: '2px solid rgba(0,0,0,0.15)',
          }}
          title={`${r.name} (${r.var_type})`}
        />
      ))}
    </BaseNode>
  );
}
