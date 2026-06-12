// TopologyView：环境架构的 React Flow 交互拓扑（替代 Mermaid 作为主展示）。
// 布局：服务器横向排列，端口/容器/进程按类型分列纵向堆叠在所属服务器下方。
// 交互：hover 节点高亮关联边；点击展开全屏。只读——不可拖拽连线。
import { useMemo, useState } from 'react';
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  type Node as RfNode,
  type Edge as RfEdge,
  type NodeProps,
} from '@xyflow/react';
import { Maximize2, X } from 'lucide-react';
import { Dialog } from '@/components/ui/Dialog';
import { cn } from '@/lib/cn';

// 与后端 architecture.TopologyGraph 对齐
interface TopologyData {
  environment_name?: string;
  nodes?: { id: string; kind: string; label: string; attrs?: Record<string, string> }[];
  edges?: { from: string; to: string; kind: string; label?: string }[];
  collection_errors?: string[];
}

type TopoNodeData = { label: string; kind: string; sub?: string; attrs?: Record<string, string> };

// kindStyle 各类节点的视觉编码：服务器最重，进程最轻。
const kindStyle: Record<string, { border: string; dot: string }> = {
  server: { border: 'border-ops-border-strong', dot: 'bg-ops-accent' },
  port: { border: 'border-ops-border-subtle', dot: 'bg-ops-info' },
  container: { border: 'border-ops-border-subtle', dot: 'bg-ops-success' },
  process: { border: 'border-ops-border-subtle', dot: 'bg-ops-tertiary' },
  env: { border: 'border-ops-border-strong', dot: 'bg-ops-secondary' },
};

// TopoNode 自定义节点：状态点 + 标签 + 副标题。
function TopoNode({ data }: NodeProps) {
  const d = data as TopoNodeData;
  const style = kindStyle[d.kind] ?? kindStyle['process']!;
  return (
    <div
      className={cn(
        'flex max-w-52 items-center gap-1.5 rounded-md border bg-ops-surface px-2 py-1 text-2xs text-ops-primary',
        style.border,
      )}
    >
      <span
        className={cn(
          'h-1.5 w-1.5 shrink-0 rounded-full',
          style.dot,
          d.kind === 'container' && 'animate-pulse',
        )}
      />
      <span className="min-w-0">
        <span className="block truncate">{d.label}</span>
        {d.sub ? <span className="block truncate text-ops-tertiary">{d.sub}</span> : null}
      </span>
    </div>
  );
}

const nodeTypes = { topo: TopoNode };

// 列内布局参数
const SERVER_GAP_X = 280;
const CHILD_ROW_H = 40;

// layoutTopology 把拓扑数据转成带坐标的 RF 节点与边。
// 简单分层：环境节点不渲染（标题已表达）；服务器一行排开，子节点按边关系挂在所属服务器下。
function layoutTopology(data: TopologyData): { nodes: RfNode[]; edges: RfEdge[] } {
  const rawNodes = (data.nodes ?? []).filter((n) => n.kind !== 'env');
  const rawEdges = (data.edges ?? []).filter((e) => !e.from.startsWith('env:'));

  // 子节点归属：from=server 的边决定挂载
  const parentOf = new Map<string, string>();
  for (const e of rawEdges) {
    if (e.from.startsWith('server:')) parentOf.set(e.to, e.from);
  }

  const servers = rawNodes.filter((n) => n.kind === 'server');
  const nodes: RfNode[] = [];
  servers.forEach((s, i) => {
    nodes.push({
      id: s.id,
      type: 'topo',
      position: { x: i * SERVER_GAP_X, y: 0 },
      data: { label: s.label, kind: s.kind, sub: s.attrs?.host, attrs: s.attrs } satisfies TopoNodeData,
      draggable: false,
    });
    // 该服务器的子节点按 kind 分组堆叠
    const children = rawNodes.filter((n) => parentOf.get(n.id) === s.id);
    const ordered = [
      ...children.filter((n) => n.kind === 'port'),
      ...children.filter((n) => n.kind === 'container'),
      ...children.filter((n) => n.kind === 'process'),
    ];
    ordered.forEach((c, j) => {
      nodes.push({
        id: c.id,
        type: 'topo',
        position: { x: i * SERVER_GAP_X + 16, y: 56 + j * CHILD_ROW_H },
        data: { label: c.label, kind: c.kind, sub: c.attrs?.image ?? c.attrs?.pid, attrs: c.attrs } satisfies TopoNodeData,
        draggable: false,
      });
    });
  });
  // 没挂上服务器的孤儿节点排到最后一列
  const placed = new Set(nodes.map((n) => n.id));
  rawNodes
    .filter((n) => !placed.has(n.id))
    .forEach((n, j) => {
      nodes.push({
        id: n.id,
        type: 'topo',
        position: { x: servers.length * SERVER_GAP_X, y: j * CHILD_ROW_H },
        data: { label: n.label, kind: n.kind, attrs: n.attrs } satisfies TopoNodeData,
        draggable: false,
      });
    });

  const edges: RfEdge[] = rawEdges
    .filter((e) => placed.has(e.from) || placed.has(e.to))
    .map((e) => ({
      id: `${e.from}->${e.to}`,
      source: e.from,
      target: e.to,
      style: { stroke: '#4A4A45', strokeWidth: 1 },
    }));
  return { nodes, edges };
}

export function TopologyView({ title, data }: { title: string; data: TopologyData }) {
  const [fullscreen, setFullscreen] = useState(false);
  const { nodes, edges } = useMemo(() => layoutTopology(data), [data]);
  if (nodes.length === 0) {
    return (
      <div className="rounded-md border border-ops-border-subtle bg-ops-input p-3 text-xs text-ops-tertiary">
        {title}（未采集到拓扑节点）
      </div>
    );
  }
  return (
    <div className="rounded-md border border-ops-border-subtle bg-ops-input">
      <div className="flex items-center justify-between border-b border-ops-border-subtle px-3 py-1.5">
        <span className="text-xs font-medium text-ops-primary">{title}</span>
        <button
          type="button"
          className="rounded p-1 text-ops-tertiary transition-colors duration-fast ease-ops hover:bg-ops-surface hover:text-ops-primary"
          title="全屏查看"
          aria-label="全屏查看"
          onClick={() => setFullscreen(true)}
        >
          <Maximize2 size={13} />
        </button>
      </div>
      <TopoCanvas nodes={nodes} edges={edges} height={260} />
      {data.collection_errors && data.collection_errors.length > 0 && (
        <div className="border-t border-ops-border-subtle px-3 py-1.5 text-2xs text-ops-warning">
          {data.collection_errors.length} 台主机采集失败（详见报告）
        </div>
      )}
      <Dialog
        open={fullscreen}
        onOpenChange={setFullscreen}
        title={title}
        size="fullscreen"
      >
        <TopoCanvas nodes={nodes} edges={edges} height="100%" />
      </Dialog>
    </div>
  );
}

// TopoCanvas 只读 RF 实例：hover 高亮关联边（电流视觉），其余弱化。
function TopoCanvas({
  nodes,
  edges,
  height,
}: {
  nodes: RfNode[];
  edges: RfEdge[];
  height: number | string;
}) {
  const [hovered, setHovered] = useState<string | null>(null);
  const [selected, setSelected] = useState<TopoNodeData | null>(null);
  const displayEdges = useMemo(() => {
    if (!hovered) return edges;
    return edges.map((e) =>
      e.source === hovered || e.target === hovered
        ? { ...e, animated: true, style: { stroke: '#60A5FA', strokeWidth: 1.5 } }
        : { ...e, style: { stroke: '#333331', strokeWidth: 1 } },
    );
  }, [edges, hovered]);
  return (
    <div style={{ height }} className="relative min-h-[200px]">
      <ReactFlowProvider>
        <ReactFlow
          nodes={nodes}
          edges={displayEdges}
          nodeTypes={nodeTypes}
          fitView
          minZoom={0.2}
          nodesConnectable={false}
          elementsSelectable={false}
          proOptions={{ hideAttribution: true }}
          onNodeMouseEnter={(_, n) => setHovered(n.id)}
          onNodeMouseLeave={() => setHovered(null)}
          onNodeClick={(_, n) => setSelected(n.data as TopoNodeData)}
        >
          <Background gap={20} size={1} />
        </ReactFlow>
      </ReactFlowProvider>
      {selected && <NodeDetailPopover node={selected} onClose={() => setSelected(null)} />}
    </div>
  );
}

// NodeDetailPopover 节点详情浮层：标签 + 类型 + 全部采集 attrs。
function NodeDetailPopover({ node, onClose }: { node: TopoNodeData; onClose: () => void }) {
  const entries = Object.entries(node.attrs ?? {});
  return (
    <div className="absolute right-2 top-2 z-10 max-w-64 rounded-md border border-ops-border-strong bg-ops-elevated p-2 shadow-glow-accent">
      <div className="mb-1 flex items-start justify-between gap-2">
        <span className="min-w-0 truncate text-2xs font-medium text-ops-primary">{node.label}</span>
        <button
          type="button"
          onClick={onClose}
          className="shrink-0 text-ops-tertiary transition-colors duration-fast ease-ops hover:text-ops-primary"
          aria-label="关闭"
        >
          <X size={12} />
        </button>
      </div>
      <div className="mb-1 text-2xs text-ops-tertiary">{node.kind}</div>
      {entries.length > 0 ? (
        <dl className="space-y-0.5 font-mono text-2xs">
          {entries.map(([k, v]) => (
            <div key={k} className="flex gap-2">
              <dt className="shrink-0 text-ops-tertiary">{k}</dt>
              <dd className="min-w-0 break-all text-ops-secondary">{v}</dd>
            </div>
          ))}
        </dl>
      ) : (
        <div className="text-2xs text-ops-tertiary">无附加信息</div>
      )}
    </div>
  );
}
