// 执行状态全局 store：内存 + Wails 事件订阅
// 数据按 FrameState 树状组织（与后端对齐）
// 事件 payload 带 framePath，reducer 递归定位到对应 frame 更新

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useReducer,
  type ReactNode,
} from 'react';
import { EventsOn } from '@wails/runtime/runtime';
import { useQueryClient } from '@tanstack/react-query';
import type {
  ExecutionRecord,
  ExecutionSnapshot,
  FrameState,
  NodeState,
  WorkflowStatus,
} from '@/types/execution';

// 单个 execution 在 store 中的状态
export interface ExecutionState {
  id: string;
  workflowID: string;
  workflowName: string;
  snapshot?: ExecutionSnapshot;
  status: WorkflowStatus;
  startedAt: string;
  finishedAt?: string;
  rootFrame: FrameState;
  error?: string;
}

interface StoreState {
  executions: Record<string, ExecutionState>;
}

// ── Wails 事件 payload 类型 ───────────────────────────────

interface StartedPayload {
  executionID: string;
  workflowID: string;
  snapshot: ExecutionSnapshot;
  startedAt: string;
}
interface StatusPayload {
  executionID: string;
  status: WorkflowStatus;
}
interface NodePayload {
  executionID: string;
  framePath: string[];
  nodeID: string;
  state: NodeState;
  errorMsg?: string;
}
interface LogPayload {
  executionID: string;
  framePath: string[];
  nodeID: string;
  time: string;
  level: 'info' | 'warn' | 'error';
  message: string;
}
interface VariablePayload {
  executionID: string;
  framePath: string[];
  name: string;
  value: unknown;
}
interface FinishedPayload {
  executionID: string;
  status: WorkflowStatus;
  error?: string;
}

// ── reducer ─────────────────────────────────────────────

type Action =
  | { type: 'started'; p: StartedPayload }
  | { type: 'status'; p: StatusPayload }
  | { type: 'node'; p: NodePayload }
  | { type: 'log'; p: LogPayload }
  | { type: 'variable'; p: VariablePayload }
  | { type: 'finished'; p: FinishedPayload }
  | { type: 'hydrate'; record: ExecutionRecord }
  | { type: 'remove'; executionID: string };

// 创建空 frame
function emptyFrame(assembleID = ''): FrameState {
  return {
    assemble_id: assembleID,
    node_states: {},
    node_logs: {},
    variables: {},
  };
}

// 沿 path 递归找到 frame；不存在时按需创建
// 返回的是新对象树（保留 React state immutability）
function withFrameAt(
  root: FrameState,
  path: string[],
  update: (f: FrameState) => FrameState,
): FrameState {
  if (path.length === 0) {
    return update(root);
  }
  const [head, ...rest] = path;
  const children = { ...(root.children ?? {}) };
  const child = children[head!] ?? emptyFrame();
  children[head!] = withFrameAt(child, rest, update);
  return { ...root, children };
}

function reducer(state: StoreState, action: Action): StoreState {
  switch (action.type) {
    case 'started': {
      const next: ExecutionState = {
        id: action.p.executionID,
        workflowID: action.p.workflowID,
        workflowName: action.p.snapshot.workflow.name,
        snapshot: action.p.snapshot,
        status: 'Running',
        startedAt: action.p.startedAt,
        rootFrame: emptyFrame(),
      };
      return {
        executions: { ...state.executions, [action.p.executionID]: next },
      };
    }
    case 'status': {
      const cur = state.executions[action.p.executionID];
      if (!cur) return state;
      return {
        executions: {
          ...state.executions,
          [action.p.executionID]: { ...cur, status: action.p.status },
        },
      };
    }
    case 'node': {
      const cur = state.executions[action.p.executionID];
      if (!cur) return state;
      const rootFrame = withFrameAt(cur.rootFrame, action.p.framePath, (f) => ({
        ...f,
        node_states: { ...f.node_states, [action.p.nodeID]: action.p.state },
      }));
      return {
        executions: {
          ...state.executions,
          [action.p.executionID]: { ...cur, rootFrame },
        },
      };
    }
    case 'log': {
      const cur = state.executions[action.p.executionID];
      if (!cur) return state;
      const rootFrame = withFrameAt(cur.rootFrame, action.p.framePath, (f) => ({
        ...f,
        node_logs: {
          ...f.node_logs,
          [action.p.nodeID]: [
            ...(f.node_logs[action.p.nodeID] ?? []),
            {
              time: action.p.time,
              level: action.p.level,
              message: action.p.message,
            },
          ],
        },
      }));
      return {
        executions: {
          ...state.executions,
          [action.p.executionID]: { ...cur, rootFrame },
        },
      };
    }
    case 'variable': {
      const cur = state.executions[action.p.executionID];
      if (!cur) return state;
      const rootFrame = withFrameAt(cur.rootFrame, action.p.framePath, (f) => ({
        ...f,
        variables: { ...f.variables, [action.p.name]: action.p.value },
      }));
      return {
        executions: {
          ...state.executions,
          [action.p.executionID]: { ...cur, rootFrame },
        },
      };
    }
    case 'finished': {
      const cur = state.executions[action.p.executionID];
      if (!cur) return state;
      return {
        executions: {
          ...state.executions,
          [action.p.executionID]: {
            ...cur,
            status: action.p.status,
            finishedAt: new Date().toISOString(),
            error: action.p.error,
          },
        },
      };
    }
    case 'hydrate': {
      const next = recordToState(action.record);
      return {
        executions: { ...state.executions, [next.id]: next },
      };
    }
    case 'remove': {
      const next = { ...state.executions };
      delete next[action.executionID];
      return { executions: next };
    }
    default:
      return state;
  }
}

// recordToState 把 API 返回的 ExecutionRecord 直接投影成 store 内的 ExecutionState
// 与 reducer 'hydrate' 分支保持一致；详情页在 store 尚未 hydrate 时用它兜底渲染，
// 避免出现「record 已到但 exec 仍为 null → 误报记录不存在」的白屏竞态。
export function recordToState(record: ExecutionRecord): ExecutionState {
  return {
    id: record.id,
    workflowID: record.workflow_id,
    workflowName: record.snapshot.workflow.name,
    snapshot: record.snapshot,
    status: record.status,
    startedAt: record.started_at,
    finishedAt: record.finished_at,
    rootFrame: record.root_frame ?? emptyFrame(),
    error: record.error,
  };
}

// 沿 path 取 frame，找不到返回 undefined
export function frameAt(
  root: FrameState | undefined,
  path: string[],
): FrameState | undefined {
  if (!root) return undefined;
  let cur: FrameState | undefined = root;
  for (const id of path) {
    cur = cur?.children?.[id];
    if (!cur) return undefined;
  }
  return cur;
}

// ── Context + Hooks ──────────────────────────────────────

interface ContextValue {
  executions: Record<string, ExecutionState>;
  hydrate: (record: ExecutionRecord) => void;
  remove: (executionID: string) => void;
}

const ExecutionStoreContext = createContext<ContextValue | null>(null);

export function ExecutionStoreProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, { executions: {} });
  const qc = useQueryClient();

  useEffect(() => {
    const invalidateList = () => {
      qc.invalidateQueries({ queryKey: ['executions'] });
    };
    const offStarted = EventsOn('execution:started', (p: StartedPayload) => {
      dispatch({ type: 'started', p });
      invalidateList();
    });
    const offStatus = EventsOn('execution:status', (p: StatusPayload) =>
      dispatch({ type: 'status', p }),
    );
    const offNode = EventsOn('execution:node', (p: NodePayload) =>
      dispatch({ type: 'node', p }),
    );
    const offLog = EventsOn('execution:log', (p: LogPayload) =>
      dispatch({ type: 'log', p }),
    );
    const offVar = EventsOn('execution:variable', (p: VariablePayload) =>
      dispatch({ type: 'variable', p }),
    );
    const offFinished = EventsOn('execution:finished', (p: FinishedPayload) => {
      dispatch({ type: 'finished', p });
      invalidateList();
    });
    return () => {
      offStarted?.();
      offStatus?.();
      offNode?.();
      offLog?.();
      offVar?.();
      offFinished?.();
    };
  }, [qc]);

  const hydrate = useCallback(
    (record: ExecutionRecord) => dispatch({ type: 'hydrate', record }),
    [],
  );
  const remove = useCallback(
    (executionID: string) => dispatch({ type: 'remove', executionID }),
    [],
  );

  const value = useMemo<ContextValue>(
    () => ({ executions: state.executions, hydrate, remove }),
    [state.executions, hydrate, remove],
  );

  return (
    <ExecutionStoreContext.Provider value={value}>
      {children}
    </ExecutionStoreContext.Provider>
  );
}

function useStore(): ContextValue {
  const ctx = useContext(ExecutionStoreContext);
  if (!ctx) throw new Error('useExecutionStore 必须在 ExecutionStoreProvider 内使用');
  return ctx;
}

export function useExecution(executionID: string | undefined): ExecutionState | null {
  const { executions } = useStore();
  if (!executionID) return null;
  return executions[executionID] ?? null;
}

export function useExecutionsByWorkflow(
  workflowID: string | undefined,
): ExecutionState[] {
  const { executions } = useStore();
  if (!workflowID) return [];
  return Object.values(executions)
    .filter((e) => e.workflowID === workflowID)
    .sort((a, b) => (a.startedAt < b.startedAt ? 1 : -1));
}

export function useAllExecutions(): ExecutionState[] {
  const { executions } = useStore();
  return Object.values(executions).sort((a, b) =>
    a.startedAt < b.startedAt ? 1 : -1,
  );
}

export function useExecutionHydrator(): {
  hydrate: (record: ExecutionRecord) => void;
  remove: (executionID: string) => void;
} {
  const { hydrate, remove } = useStore();
  return { hydrate, remove };
}
