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

// nodeDisplayTitle 集合 getter/setter 在标题中附带绑定的 param/return 名
function nodeDisplayTitle(
  data: NodeInstance,
  def: NodeTypeDef | undefined,
): string {
  const base = def?.display_name ?? data.type_id;
  if (data.type_id === 'assemble_param') {
    const name = data.config['param_name'];
    if (typeof name === 'string' && name.trim()) {
      return `${base}: ${name.trim()}`;
    }
  }
  if (data.type_id === 'return_set') {
    const name = data.config['return_name'];
    if (typeof name === 'string' && name.trim()) {
      return `${base}: ${name.trim()}`;
    }
  }
  if (data.type_id === 'var_get' || data.type_id === 'var_set') {
    const name = data.config['var_name'];
    if (typeof name === 'string' && name.trim()) {
      return `${base}: ${name.trim()}`;
    }
  }
  return base;
}

export function GenericNode({ data, selected }: Props) {
  const { data: nodeTypes } = useNodeTypes();
  const def = nodeTypes?.find((t) => t.type_id === data.type_id);
  const execState = useNodeExecState(data.instance_id);

  const inputPorts = def?.input_ports ?? [];
  const outputPorts = effectiveOutputPorts(data, def);
  const rowCount = Math.max(inputPorts.length, outputPorts.length);
  const title = nodeDisplayTitle(data, def);

  return (
    <BaseNode
      tone="neutral"
      selected={selected}
      icon={def?.icon}
      title={title}
      execState={execState}
    >
      {/* 端口行：每行最多一个 input + 一个 output；显示端口 label，类型靠 handle 颜色 + tooltip 表达 */}
      {Array.from({ length: rowCount }).map((_, idx) => {
        const inputPort = inputPorts[idx];
        const outputPort = outputPorts[idx];
        const inputText = portRowText(inputPort, data.config);
        const outputText = portRowText(outputPort, data.config);
        return (
          <div
            key={idx}
            className="flex items-center justify-between gap-3 text-[10px] text-slate-700"
            style={{ height: ROW_HEIGHT }}
          >
            <span
              className="min-w-0 flex-1 truncate pl-2.5 text-left font-medium"
              title={inputPort ? portTooltip(inputPort, data.config) : undefined}
            >
              {inputText}
            </span>
            <span
              className="min-w-0 flex-1 truncate pr-2.5 text-right font-medium"
              title={outputPort ? portTooltip(outputPort, data.config) : undefined}
            >
              {outputText}
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

// portRowText 端口行内显示的文字
// 显示 label（label 为空时回退到 port.id）；Exec 端口与未定义端口返回空字符串
function portRowText(
  port: PortDef | undefined,
  config: Record<string, unknown>,
): string {
  if (!port) return '';
  if (resolvePortType(port, config) === 'Exec') return '';
  return port.label || port.id;
}

// portTooltip handle 区文字的 hover 提示：label (Type)
function portTooltip(
  port: PortDef,
  config: Record<string, unknown>,
): string {
  return `${port.label || port.id} (${resolvePortType(port, config)})`;
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
