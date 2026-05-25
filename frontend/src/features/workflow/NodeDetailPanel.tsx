// 右侧详情面板
// - 未选中节点：显示图（工作流/集合/执行）的元信息
// - 选中节点：Config / Logs / Info 三 tab
// - 传入 executionID 时，Logs tab 从 ExecutionStore 读实时日志

import { useState, type ReactNode } from 'react';
import type { NodeInstance, VariableDef } from '@/types/workflow';
import type { ParamDef } from '@/types/assemble';
import type { GraphDef } from './canvasMapping';
import { useNodeTypes } from '@/api/nodeTypes';
import {
  ASSEMBLE_END,
  ASSEMBLE_START,
  isInternalNodeType,
  type NodeTypeDef,
} from '@/types/nodeType';
import { frameAt, useExecution } from '@/features/execution/ExecutionStore';
import { NodeStatusIcon } from '@/features/execution/ExecutionStatus';
import { ConfigForm } from './ConfigForm';
import { VariablePanel } from './VariablePanel';
import { cn } from '@/lib/cn';

interface NodeDetailPanelProps {
  graph: GraphDef & {
    id?: string;
    name?: string;
    description?: string;
    variables?: VariableDef[];
    params?: ParamDef[];
    returns?: ParamDef[];
  };
  selectedNodeId: string | null;
  // 若传入，节点的 Logs tab 显示此 execution 的实时日志
  executionID?: string;
  // 当前 frame 路径（用于从 ExecutionStore 的树状结构定位）
  framePath?: string[];
  // 节点 config 修改回调（仅编辑页传入，执行详情页不传 = 只读）
  onConfigChange?: (nodeId: string, config: Record<string, unknown>) => void;
  // 集合编辑：选中 Start/End 时在 Config 中维护 params / returns
  onParamsChange?: (items: ParamDef[]) => void;
  onReturnsChange?: (items: ParamDef[]) => void;
}

export function NodeDetailPanel({
  graph,
  selectedNodeId,
  executionID,
  framePath = [],
  onConfigChange,
  onParamsChange,
  onReturnsChange,
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
          <NodeTabs
            node={node}
            executionID={executionID}
            framePath={framePath}
            onConfigChange={onConfigChange}
            variables={graph.variables}
            params={graph.params}
            returns={graph.returns}
            onParamsChange={onParamsChange}
            onReturnsChange={onReturnsChange}
          />
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
  framePath,
  onConfigChange,
  variables,
  params,
  returns,
  onParamsChange,
  onReturnsChange,
}: {
  node: NodeInstance;
  executionID?: string;
  framePath: string[];
  onConfigChange?: (nodeId: string, config: Record<string, unknown>) => void;
  variables?: VariableDef[];
  params?: ParamDef[];
  returns?: ParamDef[];
  onParamsChange?: (items: ParamDef[]) => void;
  onReturnsChange?: (items: ParamDef[]) => void;
}) {
  const [tab, setTab] = useState<Tab>(executionID ? 'logs' : 'config');
  const exec = useExecution(executionID);
  const frame = frameAt(exec?.rootFrame, framePath);
  const nodeState = frame?.node_states[node.instance_id];
  const { data: nodeTypes } = useNodeTypes();
  const nodeType = nodeTypes?.find((t) => t.type_id === node.type_id);

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
        {tab === 'config' && (
          <ConfigTab
            key={node.instance_id}
            node={node}
            nodeType={nodeType}
            onConfigChange={onConfigChange}
            variables={variables}
            params={params}
            returns={returns}
            onParamsChange={onParamsChange}
            onReturnsChange={onReturnsChange}
          />
        )}
        {tab === 'logs' && (
          <LogsTab
            nodeID={node.instance_id}
            executionID={executionID}
            framePath={framePath}
          />
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

// ── Config tab：根据 ConfigSchema 渲染表单 ────────────────

function ConfigTab({
  node,
  nodeType,
  onConfigChange,
  variables,
  params,
  returns,
  onParamsChange,
  onReturnsChange,
}: {
  node: NodeInstance;
  nodeType: NodeTypeDef | undefined;
  onConfigChange?: (nodeId: string, config: Record<string, unknown>) => void;
  variables?: VariableDef[];
  params?: ParamDef[];
  returns?: ParamDef[];
  onParamsChange?: (items: ParamDef[]) => void;
  onReturnsChange?: (items: ParamDef[]) => void;
}) {
  // Start / End：在节点详情里直接编辑 params / returns（与左侧栏同步，并反映到节点端口）
  if (node.type_id === ASSEMBLE_START && onParamsChange) {
    return (
      <div className="-mx-4 -mt-3">
        <p className="mb-2 px-4 text-xs text-slate-500">
          入参定义会显示在 Start 节点的输出端口上；也可从左侧 Params 拖入 Get 参数节点。
        </p>
        <VariablePanel
          title="Params"
          items={params ?? []}
          onChange={onParamsChange}
          dragPayload={(item) => ({
            type_id: 'assemble_param',
            config: { param_name: item.name, var_type: item.var_type },
          })}
        />
      </div>
    );
  }
  if (node.type_id === ASSEMBLE_END && onReturnsChange) {
    return (
      <div className="-mx-4 -mt-3">
        <p className="mb-2 px-4 text-xs text-slate-500">
          返回值定义会显示在 End 节点的输入端口上；也可从左侧 Returns 拖入 Set 返回值节点。
        </p>
        <VariablePanel
          title="Returns"
          items={returns ?? []}
          onChange={onReturnsChange}
          dragPayload={(item) => ({
            type_id: 'return_set',
            config: { return_name: item.name, var_type: item.var_type },
          })}
        />
      </div>
    );
  }

  const schema = nodeType?.config_schema ?? [];
  if (schema.length === 0) {
    return <div className="text-xs text-slate-400">（无配置项）</div>;
  }
  // 没有 onConfigChange = 只读模式（执行详情页）
  if (!onConfigChange) {
    return (
      <pre className="max-h-full overflow-auto rounded bg-slate-50 p-2 text-xs">
        {JSON.stringify(node.config, null, 2)}
      </pre>
    );
  }
  return (
    <ConfigForm
      schema={schema}
      value={node.config}
      onChange={(next) => onConfigChange(node.instance_id, next)}
      variables={variables}
      params={params}
      returns={returns}
    />
  );
}

// ── Logs tab：当前节点的执行日志（来自 ExecutionStore） ──

function LogsTab({
  nodeID,
  executionID,
  framePath,
}: {
  nodeID: string;
  executionID?: string;
  framePath: string[];
}) {
  const exec = useExecution(executionID);
  if (!executionID) {
    return (
      <div className="text-xs text-slate-400">
        （仅在执行详情页可查看实时日志）
      </div>
    );
  }
  const frame = frameAt(exec?.rootFrame, framePath);
  const logs = frame?.node_logs[nodeID] ?? [];
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
