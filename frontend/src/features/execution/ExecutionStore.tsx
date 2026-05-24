// 执行状态全局 store：内存 + Wails 事件订阅
// 设计要点：
//   - 后端持续推 execution:* 事件，store 内 reducer 更新对应 ExecutionState
//   - 列表/详情页通过 hooks 读 store，保证实时性
//   - store 不与 TanStack Query 重复：API hooks 用于初次拉取，事件用于增量更新

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
  LogEntry,
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
  nodeStates: Record<string, NodeState>;
  nodeLogs: Record<string, LogEntry[]>;
  variables: Record<string, unknown>;
  error?: string;
}

// reducer state：以 executionID 索引
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
  nodeID: string;
  state: NodeState;
  errorMsg?: string;
}
interface LogPayload {
  executionID: string;
  nodeID: string;
  time: string;
  level: 'info' | 'warn' | 'error';
  message: string;
}
interface VariablePayload {
  executionID: string;
  name: string;
  value: unknown;
}
interface FinishedPayload {
  executionID: string;
  status: WorkflowStatus;
  error?: string;
}

// ── reducer actions ──────────────────────────────────────

type Action =
  | { type: 'started'; p: StartedPayload }
  | { type: 'status'; p: StatusPayload }
  | { type: 'node'; p: NodePayload }
  | { type: 'log'; p: LogPayload }
  | { type: 'variable'; p: VariablePayload }
  | { type: 'finished'; p: FinishedPayload }
  | { type: 'hydrate'; record: ExecutionRecord } // 从后端 GetExecution 注入
  | { type: 'remove'; executionID: string };

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
        nodeStates: {},
        nodeLogs: {},
        variables: {},
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
      return {
        executions: {
          ...state.executions,
          [action.p.executionID]: {
            ...cur,
            nodeStates: {
              ...cur.nodeStates,
              [action.p.nodeID]: action.p.state,
            },
          },
        },
      };
    }
    case 'log': {
      const cur = state.executions[action.p.executionID];
      if (!cur) return state;
      const prev = cur.nodeLogs[action.p.nodeID] ?? [];
      return {
        executions: {
          ...state.executions,
          [action.p.executionID]: {
            ...cur,
            nodeLogs: {
              ...cur.nodeLogs,
              [action.p.nodeID]: [
                ...prev,
                {
                  time: action.p.time,
                  level: action.p.level,
                  message: action.p.message,
                },
              ],
            },
          },
        },
      };
    }
    case 'variable': {
      const cur = state.executions[action.p.executionID];
      if (!cur) return state;
      return {
        executions: {
          ...state.executions,
          [action.p.executionID]: {
            ...cur,
            variables: {
              ...cur.variables,
              [action.p.name]: action.p.value,
            },
          },
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
      const r = action.record;
      const next: ExecutionState = {
        id: r.id,
        workflowID: r.workflow_id,
        workflowName: r.snapshot.workflow.name,
        snapshot: r.snapshot,
        status: r.status,
        startedAt: r.started_at,
        finishedAt: r.finished_at,
        nodeStates: r.node_states ?? {},
        nodeLogs: r.node_logs ?? {},
        variables: r.variables ?? {},
        error: r.error,
      };
      return {
        executions: { ...state.executions, [r.id]: next },
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

  // 订阅 6 个 Wails 事件
  // started / finished 同步 invalidate TanStack Query 的列表缓存
  // 让 useExecutions / useExecutionsByWorkflow 能从后端拉到最新混合数据
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

// useExecutionStore 拿到整个 store
function useStore(): ContextValue {
  const ctx = useContext(ExecutionStoreContext);
  if (!ctx) throw new Error('useExecutionStore 必须在 ExecutionStoreProvider 内使用');
  return ctx;
}

// useExecution 取单个 execution 的实时状态
export function useExecution(executionID: string | undefined): ExecutionState | null {
  const { executions } = useStore();
  if (!executionID) return null;
  return executions[executionID] ?? null;
}

// useExecutionsByWorkflow 取某工作流的所有 execution（按开始时间倒序）
export function useExecutionsByWorkflow(
  workflowID: string | undefined,
): ExecutionState[] {
  const { executions } = useStore();
  if (!workflowID) return [];
  return Object.values(executions)
    .filter((e) => e.workflowID === workflowID)
    .sort((a, b) => (a.startedAt < b.startedAt ? 1 : -1));
}

// useAllExecutions 取全部 execution（首页第三 tab 用）
export function useAllExecutions(): ExecutionState[] {
  const { executions } = useStore();
  return Object.values(executions).sort((a, b) =>
    a.startedAt < b.startedAt ? 1 : -1,
  );
}

// useExecutionHydrator hydrate / remove 的快捷出口
export function useExecutionHydrator(): {
  hydrate: (record: ExecutionRecord) => void;
  remove: (executionID: string) => void;
} {
  const { hydrate, remove } = useStore();
  return { hydrate, remove };
}
