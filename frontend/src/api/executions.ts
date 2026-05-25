// 执行 API hooks（TanStack Query + Wails 绑定）
// 注意：这里只包装 Wails 调用；实时事件流由 ExecutionStore 处理

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';
import {
  RunWorkflow,
  StopWorkflow,
  ListExecutions,
  ListExecutionsByWorkflow,
  GetExecution,
  DeleteExecution,
} from '@wails/go/main/App';
import type { ExecutionRecord, ExecutionSummary } from '@/types/execution';

const KEY = {
  list: ['executions'] as const,
  byWorkflow: (id: string) => ['executions', 'workflow', id] as const,
  detail: (id: string) => ['executions', id] as const,
};

// useExecutions 列出所有执行（首页第三 tab）
export function useExecutions(): UseQueryResult<ExecutionSummary[]> {
  return useQuery({
    queryKey: KEY.list,
    queryFn: () => ListExecutions() as Promise<ExecutionSummary[]>,
  });
}

// useExecutionsByWorkflow 列出某工作流的执行（RunningBadge）
export function useExecutionsByWorkflow(
  workflowID: string | undefined,
): UseQueryResult<ExecutionSummary[]> {
  return useQuery({
    queryKey: workflowID ? KEY.byWorkflow(workflowID) : ['executions', 'wf-undef'],
    queryFn: () =>
      ListExecutionsByWorkflow(workflowID!) as Promise<ExecutionSummary[]>,
    enabled: !!workflowID,
  });
}

// useExecution 详情页用，初始拉取一次；后续靠 Wails 事件更新 ExecutionStore
export function useExecution(
  executionID: string | undefined,
): UseQueryResult<ExecutionRecord> {
  return useQuery({
    queryKey: executionID ? KEY.detail(executionID) : ['executions', 'undef'],
    queryFn: () => GetExecution(executionID!) as unknown as Promise<ExecutionRecord>,
    enabled: !!executionID,
  });
}

// useRunWorkflow 启动一次执行
export function useRunWorkflow(): UseMutationResult<string, Error, string> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (workflowID) => RunWorkflow(workflowID),
    onSuccess: (_, workflowID) => {
      // 刷新列表（运行中也会被列出）
      qc.invalidateQueries({ queryKey: KEY.list });
      qc.invalidateQueries({ queryKey: KEY.byWorkflow(workflowID) });
    },
  });
}

// useStopExecution 取消执行
export function useStopExecution(): UseMutationResult<void, Error, string> {
  return useMutation({
    mutationFn: (executionID) => StopWorkflow(executionID),
  });
}

// useDeleteExecution 从内存移除执行记录
export function useDeleteExecution(): UseMutationResult<void, Error, string> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (executionID) => DeleteExecution(executionID),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: KEY.list });
    },
  });
}
