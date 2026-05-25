// 调用栈侧栏：把 ExecutionState.rootFrame 渲染成可点击的树
// 用户通过它在「主流」与各个集合 frame 之间切换，等价于面包屑下钻
//
// 与 ExecutionDetailPage 的耦合：
//   - 接收当前 framePath / selectedNodeId
//   - 点击 frame → onSelectFrame(path)（详情页内部会顺带清掉 selectedNodeId）
//   - 点击节点条目 → 先 onSelectFrame(path) 再 onSelectNode(nodeId)
//
// 不负责修改工作流结构、不重新执行。

import { useMemo, useState } from 'react';
import { useNodeTypes } from '@/api/nodeTypes';
import type { NodeTypeDef } from '@/types/nodeType';
import { cn } from '@/lib/cn';
import type { ExecutionState } from './ExecutionStore';
import {
  buildCallStackTree,
  type CallStackNode,
  type CallStackNodeEntry,
} from './buildCallStackTree';
import { NodeStatusIcon, WorkflowStatusIcon } from './ExecutionStatus';

interface Props {
  exec: ExecutionState;
  framePath: string[];
  selectedNodeId: string | null;
  onSelectFrame: (path: string[]) => void;
  onSelectNode: (nodeId: string | null) => void;
}

// 序列化 framePath 用作 React key / 比较；用 \0 避免与 instance_id 冲突
function pathKey(path: string[]): string {
  return path.join('\0');
}

export function ExecutionCallStack({
  exec,
  framePath,
  selectedNodeId,
  onSelectFrame,
  onSelectNode,
}: Props) {
  const { data: nodeTypes } = useNodeTypes();
  const nameMap = useMemo(() => buildNameMap(nodeTypes), [nodeTypes]);

  const tree = useMemo(() => {
    if (!exec.snapshot) return null;
    return buildCallStackTree(exec.rootFrame, exec.snapshot, nameMap);
  }, [exec.rootFrame, exec.snapshot, nameMap]);

  const activePathKey = pathKey(framePath);

  return (
    <aside className="flex h-full w-64 flex-col border-r border-slate-200 bg-white">
      <header className="border-b border-slate-200 px-3 py-2">
        <h2 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
          调用栈
        </h2>
      </header>
      <div className="flex-1 overflow-y-auto py-1">
        {tree ? (
          <FrameRow
            node={tree}
            depth={0}
            activePathKey={activePathKey}
            selectedNodeId={selectedNodeId}
            onSelectFrame={onSelectFrame}
            onSelectNode={onSelectNode}
          />
        ) : (
          <div className="px-3 py-2 text-xs text-slate-400">（无快照）</div>
        )}
      </div>
    </aside>
  );
}

// 从 NodeTypeDef[] 构造 type_id → display_name 映射
function buildNameMap(
  defs: NodeTypeDef[] | undefined,
): Map<string, string> | undefined {
  if (!defs || defs.length === 0) return undefined;
  const m = new Map<string, string>();
  for (const d of defs) m.set(d.type_id, d.display_name);
  return m;
}

// 单个 frame 行 + 其 nodeEntries + 递归子 frame
function FrameRow({
  node,
  depth,
  activePathKey,
  selectedNodeId,
  onSelectFrame,
  onSelectNode,
}: {
  node: CallStackNode;
  depth: number;
  activePathKey: string;
  selectedNodeId: string | null;
  onSelectFrame: (path: string[]) => void;
  onSelectNode: (nodeId: string | null) => void;
}) {
  const [expanded, setExpanded] = useState(true);
  const isActive = pathKey(node.framePath) === activePathKey;
  const hasContent = node.children.length > 0 || node.nodeEntries.length > 0;

  return (
    <div>
      <div
        className={cn(
          'flex items-center gap-1.5 px-1 py-1 text-xs',
          isActive && 'bg-blue-50',
        )}
        style={{ paddingLeft: 6 + depth * 12 }}
      >
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className={cn(
            'flex h-4 w-4 shrink-0 items-center justify-center text-slate-400 hover:text-slate-600',
            !hasContent && 'invisible',
          )}
          aria-label={expanded ? '折叠' : '展开'}
        >
          {expanded ? '▾' : '▸'}
        </button>
        <button
          type="button"
          onClick={() => {
            onSelectFrame(node.framePath);
            onSelectNode(null);
          }}
          className={cn(
            'flex min-w-0 flex-1 items-center gap-1.5 truncate rounded px-1 py-0.5 text-left',
            isActive
              ? 'font-medium text-slate-900'
              : 'text-slate-700 hover:bg-slate-100',
          )}
          title={node.label}
        >
          <WorkflowStatusIcon status={node.status} size={12} />
          <span className="truncate">
            {node.kind === 'assemble' ? '📦 ' : ''}
            {node.label}
          </span>
        </button>
      </div>
      {expanded && (
        <>
          {node.nodeEntries.length > 0 && (
            <ul>
              {node.nodeEntries.map((entry) => (
                <NodeEntryRow
                  key={entry.nodeId}
                  entry={entry}
                  depth={depth + 1}
                  framePath={node.framePath}
                  isActiveFrame={isActive}
                  selectedNodeId={selectedNodeId}
                  onSelectFrame={onSelectFrame}
                  onSelectNode={onSelectNode}
                />
              ))}
            </ul>
          )}
          {node.children.map((child) => (
            <FrameRow
              key={pathKey(child.framePath)}
              node={child}
              depth={depth + 1}
              activePathKey={activePathKey}
              selectedNodeId={selectedNodeId}
              onSelectFrame={onSelectFrame}
              onSelectNode={onSelectNode}
            />
          ))}
        </>
      )}
    </div>
  );
}

function NodeEntryRow({
  entry,
  depth,
  framePath,
  isActiveFrame,
  selectedNodeId,
  onSelectFrame,
  onSelectNode,
}: {
  entry: CallStackNodeEntry;
  depth: number;
  framePath: string[];
  isActiveFrame: boolean;
  selectedNodeId: string | null;
  onSelectFrame: (path: string[]) => void;
  onSelectNode: (nodeId: string | null) => void;
}) {
  const isSelected = isActiveFrame && selectedNodeId === entry.nodeId;
  return (
    <li
      className={cn(
        'flex items-center gap-1.5 px-1 py-0.5 text-xs',
        isSelected && 'bg-blue-50',
      )}
      style={{ paddingLeft: 6 + depth * 12 + 18 }}
    >
      <button
        type="button"
        onClick={() => {
          // 节点属于本 frame，先确保 frame 选中
          if (!isActiveFrame) onSelectFrame(framePath);
          onSelectNode(entry.nodeId);
        }}
        className={cn(
          'flex min-w-0 flex-1 items-center gap-1.5 truncate rounded px-1 py-0.5 text-left',
          isSelected
            ? 'font-medium text-slate-900'
            : 'text-slate-600 hover:bg-slate-100',
        )}
        title={`${entry.label} (${entry.state})`}
      >
        <NodeStatusIcon state={entry.state} size={10} />
        <span className="truncate">{entry.label}</span>
      </button>
    </li>
  );
}
