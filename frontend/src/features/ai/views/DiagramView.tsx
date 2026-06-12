// DiagramView：render_diagram 工具的通用图渲染（受控，无 HTML/SVG 注入面）。
// 自动布局：按边做拓扑分层（无环时层级清晰；有环时退化为出现顺序）。
// 节点 kind 决定配色：因果链 fact/judgment/suggestion + 通用 default/primary/warning/danger。
import { useMemo, useState } from 'react';
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  type Node as RfNode,
  type Edge as RfEdge,
  type NodeProps,
  MarkerType,
} from '@xyflow/react';
import { Maximize2 } from 'lucide-react';
import { Dialog } from '@/components/ui/Dialog';
import { cn } from '@/lib/cn';

interface DiagramSpec {
  title?: string;
  nodes?: { id: string; label: string; kind?: string }[];
  edges?: { from: string; to: string; label?: string }[];
  groups?: { label: string; nodes: string[] }[];
}

type DiagramNodeData = { label: string; kind: string };

// kindStyle：kind → 边框/底色（语义化）。未知 kind 回退 default。
const kindStyle: Record<string, string> = {
  fact: 'border-ops-info/60 bg-ops-info-soft text-ops-primary',
  judgment: 'border-ops-warning/60 bg-ops-warning-soft text-ops-primary',
  suggestion: 'border-ops-success/60 bg-ops-success-soft text-ops-primary',
  primary: 'border-ops-accent/60 bg-ops-accent-soft text-ops-primary',
  warning: 'border-ops-warning/60 bg-ops-warning-soft text-ops-primary',
  danger: 'border-ops-danger/60 bg-ops-danger-soft text-ops-primary',
  default: 'border-ops-border-strong bg-ops-surface text-ops-primary',
};

function DiagramNode({ data }: NodeProps) {
  const d = data as DiagramNodeData;
  const cls = kindStyle[d.kind] ?? kindStyle['default']!;
  return (
    <div className={cn('max-w-56 rounded-md border px-2.5 py-1.5 text-2xs leading-snug', cls)}>
      {d.label}
    </div>
  );
}

const nodeTypes = { diagram: DiagramNode };

const LAYER_GAP_X = 220;
const ROW_GAP_Y = 64;

// layoutDiagram 拓扑分层：入度为 0 的节点在第 0 层，其余取「所有前驱最大层 + 1」。
// 有环时未排入的节点兜底追加到末层，保证全部可见。
function layoutDiagram(spec: DiagramSpec): { nodes: RfNode[]; edges: RfEdge[] } {
  const raw = spec.nodes ?? [];
  const edges = spec.edges ?? [];
  const ids = new Set(raw.map((n) => n.id));
  const indeg = new Map<string, number>();
  const adj = new Map<string, string[]>();
  raw.forEach((n) => indeg.set(n.id, 0));
  edges.forEach((e) => {
    if (!ids.has(e.from) || !ids.has(e.to)) return;
    indeg.set(e.to, (indeg.get(e.to) ?? 0) + 1);
    adj.set(e.from, [...(adj.get(e.from) ?? []), e.to]);
  });

  // Kahn 分层
  const layer = new Map<string, number>();
  let frontier = raw.filter((n) => (indeg.get(n.id) ?? 0) === 0).map((n) => n.id);
  frontier.forEach((id) => layer.set(id, 0));
  const remaining = new Map(indeg);
  let cur = frontier;
  while (cur.length > 0) {
    const next: string[] = [];
    for (const u of cur) {
      for (const v of adj.get(u) ?? []) {
        remaining.set(v, (remaining.get(v) ?? 0) - 1);
        layer.set(v, Math.max(layer.get(v) ?? 0, (layer.get(u) ?? 0) + 1));
        if ((remaining.get(v) ?? 0) === 0) next.push(v);
      }
    }
    cur = next;
  }
  // 环中节点（未定层）兜底
  const maxLayer = Math.max(0, ...Array.from(layer.values()));
  raw.forEach((n) => {
    if (!layer.has(n.id)) layer.set(n.id, maxLayer + 1);
  });

  // 同层内按出现顺序排行
  const rowOf = new Map<number, number>();
  const nodes: RfNode[] = raw.map((n) => {
    const l = layer.get(n.id) ?? 0;
    const row = rowOf.get(l) ?? 0;
    rowOf.set(l, row + 1);
    return {
      id: n.id,
      type: 'diagram',
      position: { x: l * LAYER_GAP_X, y: row * ROW_GAP_Y },
      data: { label: n.label, kind: n.kind ?? 'default' } satisfies DiagramNodeData,
      draggable: false,
    };
  });

  const rfEdges: RfEdge[] = edges
    .filter((e) => ids.has(e.from) && ids.has(e.to))
    .map((e, i) => ({
      id: `${e.from}->${e.to}-${i}`,
      source: e.from,
      target: e.to,
      label: e.label,
      labelStyle: { fill: '#A8A6A1', fontSize: 10 },
      style: { stroke: '#6B6962', strokeWidth: 1.2 },
      markerEnd: { type: MarkerType.ArrowClosed, color: '#6B6962' },
    }));
  return { nodes, edges: rfEdges };
}

export function DiagramView({ title, data }: { title: string; data: DiagramSpec }) {
  const [fullscreen, setFullscreen] = useState(false);
  const { nodes, edges } = useMemo(() => layoutDiagram(data), [data]);
  if (nodes.length === 0) {
    return (
      <div className="rounded-md border border-ops-border-subtle bg-ops-input p-3 text-xs text-ops-tertiary">
        {title}（空图）
      </div>
    );
  }
  return (
    <div className="rounded-md border border-ops-border-subtle bg-ops-input">
      <div className="flex items-center justify-between border-b border-ops-border-subtle px-3 py-1.5">
        <span className="min-w-0 truncate text-xs font-medium text-ops-primary">{title}</span>
        <button
          type="button"
          className="shrink-0 rounded p-1 text-ops-tertiary transition-colors duration-fast ease-ops hover:bg-ops-surface hover:text-ops-primary"
          title="全屏查看"
          aria-label="全屏查看"
          onClick={() => setFullscreen(true)}
        >
          <Maximize2 size={13} />
        </button>
      </div>
      <DiagramCanvas nodes={nodes} edges={edges} height={Math.min(360, 80 + nodes.length * 30)} />
      <Dialog open={fullscreen} onOpenChange={setFullscreen} title={title} size="fullscreen">
        <DiagramCanvas nodes={nodes} edges={edges} height="100%" />
      </Dialog>
    </div>
  );
}

function DiagramCanvas({
  nodes,
  edges,
  height,
}: {
  nodes: RfNode[];
  edges: RfEdge[];
  height: number | string;
}) {
  return (
    <div style={{ height }} className="min-h-[180px]">
      <ReactFlowProvider>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          fitView
          minZoom={0.2}
          nodesConnectable={false}
          elementsSelectable={false}
          proOptions={{ hideAttribution: true }}
        >
          <Background gap={20} size={1} />
        </ReactFlow>
      </ReactFlowProvider>
    </div>
  );
}
