// DiagramView：render_diagram 工具的通用图渲染（受控，无 HTML/SVG 注入面）。
// 布局：dagre 自动分层（默认垂直流向 TB），产出 Claude 式清晰流程图。
// 节点 kind 决定配色：因果链 fact/judgment/suggestion + 通用 default/primary/warning/danger。
import { useMemo, useState } from 'react';
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Handle,
  Position,
  type Node as RfNode,
  type Edge as RfEdge,
  type NodeProps,
  MarkerType,
} from '@xyflow/react';
import Dagre from '@dagrejs/dagre';
import { Maximize2 } from 'lucide-react';
import { Dialog } from '@/components/ui/Dialog';
import { cn } from '@/lib/cn';

interface DiagramSpec {
  title?: string;
  // direction：TB 垂直（默认，流程图）/ LR 水平。模型可选。
  direction?: 'TB' | 'LR';
  nodes?: { id: string; label: string; kind?: string }[];
  edges?: { from: string; to: string; label?: string }[];
  groups?: { label: string; nodes: string[] }[];
}

type DiagramNodeData = { label: string; kind: string; horizontal: boolean };

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

// DiagramNode：自定义节点必须声明 Handle，否则边无法连接（之前一条线都不显示的根因）。
// Handle 透明无交互（只读图），位置随流向：垂直流程 = 上入下出，水平 = 左入右出。
function DiagramNode({ data }: NodeProps) {
  const d = data as DiagramNodeData;
  const cls = kindStyle[d.kind] ?? kindStyle['default']!;
  const targetPos = d.horizontal ? Position.Left : Position.Top;
  const sourcePos = d.horizontal ? Position.Right : Position.Bottom;
  return (
    <>
      <Handle type="target" position={targetPos} className="!h-1 !w-1 !border-0 !bg-transparent" />
      <div className={cn('w-44 rounded-md border px-2.5 py-1.5 text-center text-2xs leading-snug', cls)}>
        {d.label}
      </div>
      <Handle type="source" position={sourcePos} className="!h-1 !w-1 !border-0 !bg-transparent" />
    </>
  );
}

const nodeTypes = { diagram: DiagramNode };

// 节点估算尺寸（dagre 布局用；与 DiagramNode 的 w-44 + padding 对齐）
const NODE_W = 176;
const NODE_H = 44;

// layoutDiagram 用 dagre 计算分层布局，产出带坐标的 RF 节点与边。
function layoutDiagram(spec: DiagramSpec): {
  nodes: RfNode[];
  edges: RfEdge[];
  horizontal: boolean;
} {
  const raw = spec.nodes ?? [];
  const rawEdges = spec.edges ?? [];
  const ids = new Set(raw.map((n) => n.id));
  const horizontal = spec.direction === 'LR';

  const g = new Dagre.graphlib.Graph().setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: horizontal ? 'LR' : 'TB', ranksep: 56, nodesep: 28, marginx: 12, marginy: 12 });
  raw.forEach((n) => g.setNode(n.id, { width: NODE_W, height: NODE_H }));
  const validEdges = rawEdges.filter((e) => ids.has(e.from) && ids.has(e.to));
  validEdges.forEach((e) => g.setEdge(e.from, e.to));
  Dagre.layout(g);

  const nodes: RfNode[] = raw.map((n) => {
    const pos = g.node(n.id);
    return {
      id: n.id,
      type: 'diagram',
      // dagre 给的是中心点，RF 用左上角
      position: { x: (pos?.x ?? 0) - NODE_W / 2, y: (pos?.y ?? 0) - NODE_H / 2 },
      data: { label: n.label, kind: n.kind ?? 'default', horizontal } satisfies DiagramNodeData,
      draggable: false,
    };
  });

  const edges: RfEdge[] = validEdges.map((e, i) => ({
    id: `${e.from}->${e.to}-${i}`,
    source: e.from,
    target: e.to,
    label: e.label,
    type: 'smoothstep',
    labelStyle: { fill: '#A8A6A1', fontSize: 10 },
    labelBgStyle: { fill: '#1A1A19', fillOpacity: 0.85 },
    style: { stroke: '#8B8A84', strokeWidth: 1.4 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#8B8A84', width: 16, height: 16 },
  }));
  return { nodes, edges, horizontal };
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
  // 高度按节点行数估算（垂直流向时层数多→更高）
  const estHeight = Math.min(420, Math.max(200, 90 + nodes.length * 36));
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
      <DiagramCanvas nodes={nodes} edges={edges} height={estHeight} />
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
          fitViewOptions={{ padding: 0.15 }}
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
