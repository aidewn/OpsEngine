// 右侧详情面板
// - 未选中节点：显示图（工作流/集合/执行）的元信息
// - 选中节点：Config / Logs / Info 三 tab
// - 传入 executionID 时，Logs tab 从 ExecutionStore 读实时日志

import { useState, type ReactNode } from 'react';
import type { NodeInstance } from '@/types/workflow';
import type { GraphDef } from './canvasMapping';
import { useNodeTypes } from '@/api/nodeTypes';
import { isInternalNodeType } from '@/types/nodeType';
import { useExecution } from '@/features/execution/ExecutionStore';
import { NodeStatusIcon } from '@/features/execution/ExecutionStatus';
import { cn } from '@/lib/cn';

interface NodeDetailPanelProps {
  graph: GraphDef & { id?: string; name?: string; description?: string };
  selectedNodeId: string | null;
  // 若传入，节点的 Logs tab 显示此 execution 的实时日志
  executionID?: string;
}

export function NodeDetailPanel({
  graph,
  selectedNodeId,
  executionID,
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
      <div className="flex-1 overflow-y-auto">
        {node ? (
          <NodeTabs node={node} executionID={executionID} />
        ) : (
          <div className="px-4 py-3">
            <GraphDetail graph={graph} />
          </div>
        )}
      </div>
    </aside>
  );
}

// ── 节点 tabs ────────────────────────────────────────────

type Tab = 'config' | 'logs' | 'info';

function NodeTabs({
  node,
  executionID,
}: {
  node: NodeInstance;
  executionID?: string;
}) {
  const [tab, setTab] = useState<Tab>(executionID ? 'logs' : 'info');
  const exec = useExecution(executionID);
  const nodeState = exec?.nodeStates[node.instance_id];

  return (
    <div className="flex h-full flex-col">
      {/* 状态指示头 */}
      {executionID && (
        <div className="flex items-center gap-2 border-b border-slate-200 bg-slate-50 px-4 py-2">
          <NodeStatusIcon state={nodeState} size={16} />
          <span className="text-xs font-medium text-slate-700">
            {nodeState ?? 'Idle'}
          </span>
        </div>
      )}

      {/* tab 切换 */}
      <div className="flex border-b border-slate-200 bg-white">
        <TabButton active={tab === 'config'} onClick={() => setTab('config')}>
          Config
        </TabButton>
        <TabButton active={tab === 'logs'} onClick={() => setTab('logs')}>
          Logs
        </TabButton>
        <TabButton active={tab === 'info'} onClick={() => setTab('info')}>
          Info
        </TabButton>
      </div>

      {/* tab 内容 */}
      <div className="flex-1 overflow-y-auto px-4 py-3">
        {tab === 'config' && <ConfigTab node={node} />}
        {tab === 'logs' && (
          <LogsTab nodeID={node.instance_id} executionID={executionID} />
        )}
        {tab === 'info' && <InfoTab node={node} />}
      </div>
    </div>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'flex-1 border-b-2 px-3 py-2 text-xs font-medium transition-colors',
        active
          ? 'border-blue-500 text-slate-900'
          : 'border-transparent text-slate-500 hover:text-slate-700',
      )}
    >
      {children}
    </button>
  );
}

// ── Config tab：当前节点的 config（MVP 阶段只读 JSON） ────

function ConfigTab({ node }: { node: NodeInstance }) {
  if (Object.keys(node.config).length === 0) {
    return <div className="text-xs text-slate-400">（空）</div>;
  }
  return (
    <pre className="max-h-full overflow-auto rounded bg-slate-50 p-2 text-xs">
      {JSON.stringify(node.config, null, 2)}
    </pre>
  );
}

// ── Logs tab：当前节点的执行日志（来自 ExecutionStore） ──

function LogsTab({
  nodeID,
  executionID,
}: {
  nodeID: string;
  executionID?: string;
}) {
  const exec = useExecution(executionID);
  if (!executionID) {
    return (
      <div className="text-xs text-slate-400">
        （仅在执行详情页可查看实时日志）
      </div>
    );
  }
  const logs = exec?.nodeLogs[nodeID] ?? [];
  if (logs.length === 0) {
    return <div className="text-xs text-slate-400">（无日志）</div>;
  }
  return (
    <div className="space-y-1 font-mono text-[11px]">
      {logs.map((log, i) => (
        <div
          key={i}
          className={cn(
            'whitespace-pre-wrap break-words',
            log.level === 'error' && 'text-red-600',
            log.level === 'warn' && 'text-amber-600',
            log.level === 'info' && 'text-slate-700',
          )}
        >
          <span className="text-slate-400">
            {formatTime(log.time)}{' '}
          </span>
          {log.message}
        </div>
      ))}
    </div>
  );
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleTimeString('zh-CN', { hour12: false });
  } catch {
    return iso;
  }
}

// ── Info tab：节点元信息 ──────────────────────────────────

function InfoTab({ node }: { node: NodeInstance }) {
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
    </dl>
  );
}

// ── 图详情（未选节点时） ────────────────────────────────

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
