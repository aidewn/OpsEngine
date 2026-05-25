// 通用业务节点：根据 NodeTypeDef 动态渲染所有端口
// 布局：按行对齐 input/output，handle 在行的左/右边缘，label 内嵌

import { Handle, Position, type NodeProps } from '@xyflow/react';
import { BaseNode } from './BaseNode';
import type { NodeInstance } from '@/types/workflow';
import type { NodeTypeDef, PortDef } from '@/types/nodeType';
import { useNodeTypes } from '@/api/nodeTypes';
import { getPortColor, resolvePortType } from '@/types/nodeType';
import { useNodeExecState } from './useNodeExecState';
import { effectiveBranchCount } from '../cleanupParallel';

type Props = NodeProps & { data: NodeInstance };

const ROW_HEIGHT = 20;
// BaseNode 标题区高度 + body padding-top（用于 handle 定位）
const HEADER_OFFSET = 30;

export function GenericNode({ data, selected }: Props) {
  const { data: nodeTypes } = useNodeTypes();
  const def = nodeTypes?.find((t) => t.type_id === data.type_id);
  const execState = useNodeExecState(data.instance_id);

  const inputPorts = def?.input_ports ?? [];
  // parallel 节点根据 config.branch_count 动态计算可见输出端口数
  const outputPorts = effectiveOutputPorts(data, def);
  const rowCount = Math.max(inputPorts.length, outputPorts.length);

  return (
    <BaseNode
      tone="neutral"
      selected={selected}
      icon={def?.icon}
      title={def?.display_name ?? data.type_id}
      execState={execState}
    >
      {/* 端口行：每行最多一个 input + 一个 output；显示「类型」而非「名字」 */}
      {Array.from({ length: rowCount }).map((_, idx) => {
        const inputPort = inputPorts[idx];
        const outputPort = outputPorts[idx];
        const inputType = inputPort
          ? resolvePortType(inputPort, data.config)
          : null;
        const outputType = outputPort
          ? resolvePortType(outputPort, data.config)
          : null;
        return (
          <div
            key={idx}
            className="flex items-center justify-between text-[10px] text-slate-700"
            style={{ height: ROW_HEIGHT }}
          >
            <span className="truncate pl-2.5 font-medium">
              {inputType && inputType !== 'Exec' ? inputType : ''}
            </span>
            <span className="truncate pr-2.5 font-medium">
              {outputType && outputType !== 'Exec' ? outputType : ''}
            </span>
          </div>
        );
      })}

      {/* Handles 绝对定位在每行的左右边缘 */}
      {inputPorts.map((port, idx) => (
        <PortHandle
          key={`in-${port.id}`}
          port={port}
          config={data.config}
          side="left"
          rowIndex={idx}
        />
      ))}
      {outputPorts.map((port, idx) => (
        <PortHandle
          key={`out-${port.id}`}
          port={port}
          config={data.config}
          side="right"
          rowIndex={idx}
        />
      ))}
    </BaseNode>
  );
}

// effectiveOutputPorts 计算节点实际可见的输出端口列表
// parallel 节点：根据 config.branch_count 只显示前 N 个 exec_out_<i> + exec_out_done
// 其他节点：使用 TypeDef 中定义的全部 output_ports
function effectiveOutputPorts(
  data: NodeInstance,
  def: NodeTypeDef | undefined,
): PortDef[] {
  const all = def?.output_ports ?? [];
  if (data.type_id !== 'parallel') return all;
  const n = effectiveBranchCount(data.config);
  return all.filter((p) => {
    const m = /^exec_out_(\d+)$/.exec(p.id);
    if (!m) return true; // exec_out_done 保留
    const idx = parseInt(m[1] ?? '0', 10);
    return idx <= n;
  });
}

// 单个 handle，绝对定位到行的左/右边缘
function PortHandle({
  port,
  config,
  side,
  rowIndex,
}: {
  port: PortDef;
  config: Record<string, unknown>;
  side: 'left' | 'right';
  rowIndex: number;
}) {
  const realType = resolvePortType(port, config);
  const color = getPortColor(realType);
  const isExec = realType === 'Exec';

  return (
    <Handle
      type={side === 'left' ? 'target' : 'source'}
      position={side === 'left' ? Position.Left : Position.Right}
      id={port.id}
      style={{
        top: HEADER_OFFSET + rowIndex * ROW_HEIGHT + ROW_HEIGHT / 2,
        background: color,
        width: isExec ? 12 : 14,
        height: isExec ? 12 : 14,
        borderRadius: isExec ? 2 : 6,
        border: '2px solid rgba(0,0,0,0.15)',
      }}
      title={`${port.label} (${realType})`}
    />
  );
}
