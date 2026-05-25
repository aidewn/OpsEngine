// 通用业务节点：根据 NodeTypeDef 动态渲染所有端口
// 布局：按行对齐 input/output，handle 在行的左/右边缘，label 内嵌
// 副标题：当 nodeSummary 注册了该 type 时显示 config 摘要（cmd 首行 / 算术表达式…）
// 默认值 chip：未连线的非 Exec 数据 input 旁显示 config 里配的 default 值

import {
  Handle,
  Position,
  useNodeConnections,
  type NodeProps,
} from '@xyflow/react';
import { BaseNode } from './BaseNode';
import type { NodeInstance } from '@/types/workflow';
import type { NodeTypeDef, PortDef, PortType } from '@/types/nodeType';
import { useNodeTypes } from '@/api/nodeTypes';
import { getPortColor, resolvePortType } from '@/types/nodeType';
import { useNodeExecState } from './useNodeExecState';
import { effectiveBranchCount } from '../cleanupParallel';
import { nodeSummary } from './nodeSummary';

type Props = NodeProps & { data: NodeInstance };

const ROW_HEIGHT = 20;
// BaseNode 标题区高度：无副标题 30px，有副标题需多挤一行 (~12px)
const HEADER_OFFSET_BASE = 30;
const HEADER_OFFSET_WITH_SUBTITLE = 42;

// 默认值 chip 仅对这些可读类型显示；句柄类型（LinuxSshConnection 等）必须连线，跳过
const INLINEABLE_TYPES: ReadonlySet<PortType> = new Set([
  'String',
  'Int',
  'Float',
  'Bool',
  'Dynamic',
  'Any',
]);

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

export function GenericNode({ id, data, selected }: Props) {
  const { data: nodeTypes } = useNodeTypes();
  const def = nodeTypes?.find((t) => t.type_id === data.type_id);
  const execState = useNodeExecState(data.instance_id);

  const inputPorts = def?.input_ports ?? [];
  const outputPorts = effectiveOutputPorts(data, def);
  const rowCount = Math.max(inputPorts.length, outputPorts.length);
  const title = nodeDisplayTitle(data, def);
  const subtitle = nodeSummary(data.type_id, data.config);
  const headerOffset = subtitle ? HEADER_OFFSET_WITH_SUBTITLE : HEADER_OFFSET_BASE;

  return (
    <BaseNode
      tone="neutral"
      selected={selected}
      icon={def?.icon}
      title={title}
      subtitle={subtitle ?? undefined}
      execState={execState}
    >
      {/* 端口行：每行最多一个 input + 一个 output；显示端口 label，类型靠 handle 颜色 + tooltip 表达 */}
      {Array.from({ length: rowCount }).map((_, idx) => {
        const inputPort = inputPorts[idx];
        const outputPort = outputPorts[idx];
        return (
          <div
            key={idx}
            className="flex items-center justify-between gap-3 text-[10px] text-slate-700"
            style={{ height: ROW_HEIGHT }}
          >
            {inputPort ? (
              <InputPortCell
                nodeId={id}
                port={inputPort}
                config={data.config}
              />
            ) : (
              <span className="min-w-0 flex-1 pl-2.5" />
            )}
            <span
              className="min-w-0 flex-1 truncate pr-2.5 text-right font-medium"
              title={outputPort ? portTooltip(outputPort, data.config) : undefined}
            >
              {portRowText(outputPort, data.config)}
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
          headerOffset={headerOffset}
        />
      ))}
      {outputPorts.map((port, idx) => (
        <PortHandle
          key={`out-${port.id}`}
          port={port}
          config={data.config}
          side="right"
          rowIndex={idx}
          headerOffset={headerOffset}
        />
      ))}
    </BaseNode>
  );
}

// InputPortCell 单个 input 行内文本；未连线 + 有 default 时附带灰色 chip
// 必须独立组件——useNodeConnections 不能在循环 / 条件里调用
function InputPortCell({
  nodeId,
  port,
  config,
}: {
  nodeId: string;
  port: PortDef;
  config: Record<string, unknown>;
}) {
  const connections = useNodeConnections({
    id: nodeId,
    handleType: 'target',
    handleId: port.id,
  });
  const isUnconnected = connections.length === 0;
  const defaultStr = isUnconnected ? defaultValueFor(port, config) : null;
  const text = portRowText(port, config);
  return (
    <span
      className="flex min-w-0 flex-1 items-center gap-1 pl-2.5 text-left font-medium"
      title={portTooltip(port, config)}
    >
      <span className="truncate">{text}</span>
      {defaultStr !== null && (
        <span
          className="shrink-0 rounded bg-slate-100 px-1 py-px font-mono text-[9px] font-normal text-slate-500"
          title={`默认值（未连线时使用）：${defaultStr}`}
        >
          ={truncateDefault(defaultStr, 12)}
        </span>
      )}
    </span>
  );
}

// defaultValueFor 取该 input 端口的 config 默认值字符串；无则 null
// 约定：先看 <portId>_default（arith/compare 新约定），再看 <portId>（linux_exec_command 等老约定）
function defaultValueFor(
  port: PortDef,
  config: Record<string, unknown>,
): string | null {
  const realType = resolvePortType(port, config);
  if (realType === 'Exec') return null;
  if (!INLINEABLE_TYPES.has(realType)) return null;
  const candidates: unknown[] = [
    config[`${port.id}_default`],
    config[port.id],
  ];
  for (const c of candidates) {
    if (typeof c === 'string') {
      const t = c.trim();
      if (t) return t;
    } else if (typeof c === 'number' || typeof c === 'boolean') {
      return String(c);
    }
  }
  return null;
}

function truncateDefault(s: string, max: number): string {
  if (s.length <= max) return s;
  return s.slice(0, max - 1) + '…';
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
  headerOffset,
}: {
  port: PortDef;
  config: Record<string, unknown>;
  side: 'left' | 'right';
  rowIndex: number;
  headerOffset: number;
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
        top: headerOffset + rowIndex * ROW_HEIGHT + ROW_HEIGHT / 2,
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
