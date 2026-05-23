// 右侧详情面板
// - 未选中节点：显示工作流元信息（id / name / description）
// - 选中节点：显示节点元数据（type / instance_id / position）
//   MVP 不渲染节点 config 表单——下个迭代再做

import type { ReactNode } from 'react';
import type { NodeInstance } from '@/types/workflow';
import type { GraphDef } from './canvasMapping';
import { useNodeTypes } from '@/api/nodeTypes';
import { isInternalNodeType } from '@/types/nodeType';

interface NodeDetailPanelProps {
  graph: GraphDef & { id?: string; name?: string; description?: string };
  selectedNodeId: string | null;
}

export function NodeDetailPanel({
  graph,
  selectedNodeId,
}: NodeDetailPanelProps) {
  const node = selectedNodeId
    ? (graph.nodes.find((n) => n.instance_id === selectedNodeId) ?? null)
    : null;

  return (
    <aside className="flex h-full w-80 flex-col border-l border-slate-200 bg-white">
      <header className="border-b border-slate-200 px-4 py-3">
        <h2 className="text-sm font-semibold text-slate-900">
          {node ? '节点详情' : '详情'}
        </h2>
      </header>
      <div className="flex-1 overflow-y-auto px-4 py-3">
        {node ? (
          <NodeDetail node={node} />
        ) : (
          <GraphDetail graph={graph} />
        )}
      </div>
    </aside>
  );
}

function GraphDetail({
  graph,
}: {
  graph: GraphDef & { id?: string; name?: string; description?: string };
}) {
  return (
    <dl className="space-y-3 text-sm">
      {graph.id && <Field label="ID" value={graph.id} mono />}
      {graph.name && <Field label="名称" value={graph.name} />}
      <Field
        label="描述"
        value={
          graph.description || (
            <span className="text-slate-400">（未填写）</span>
          )
        }
      />
      <Field label="节点数量" value={String(graph.nodes.length)} />
      <Field label="连线数量" value={String(graph.edges.length)} />
    </dl>
  );
}

function NodeDetail({ node }: { node: NodeInstance }) {
  const { data: nodeTypes } = useNodeTypes();
  const def = nodeTypes?.find((t) => t.type_id === node.type_id);
  const isInternal = isInternalNodeType(node.type_id);

  return (
    <dl className="space-y-3 text-sm">
      <Field
        label="类型"
        value={
          <span>
            {def?.display_name ?? node.type_id}
            {isInternal && (
              <span className="ml-1 rounded bg-slate-100 px-1 py-0.5 text-[10px] uppercase text-slate-600">
                internal
              </span>
            )}
          </span>
        }
      />
      <Field label="Type ID" value={node.type_id} mono />
      <Field label="实例 ID" value={node.instance_id} mono />
      <Field
        label="坐标"
        value={`(${node.position.x.toFixed(0)}, ${node.position.y.toFixed(0)})`}
        mono
      />
      <Field
        label="Config"
        value={
          Object.keys(node.config).length === 0 ? (
            <span className="text-slate-400">（空）</span>
          ) : (
            <pre className="mt-1 max-h-48 overflow-auto rounded bg-slate-50 p-2 text-xs">
              {JSON.stringify(node.config, null, 2)}
            </pre>
          )
        }
      />
    </dl>
  );
}

function Field({
  label,
  value,
  mono,
}: {
  label: string;
  value: ReactNode;
  mono?: boolean;
}) {
  return (
    <div>
      <dt className="text-xs font-medium uppercase tracking-wide text-slate-500">
        {label}
      </dt>
      <dd
        className={
          mono
            ? 'mt-1 break-all font-mono text-xs text-slate-800'
            : 'mt-1 text-slate-800'
        }
      >
        {value}
      </dd>
    </div>
  );
}
