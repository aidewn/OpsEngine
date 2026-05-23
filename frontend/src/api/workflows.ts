// 工作流相关 API hooks（TanStack Query + Wails 绑定）

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';
import {
  ListWorkflows,
  GetWorkflow,
  CreateWorkflow,
  UpdateWorkflow,
  DeleteWorkflow,
} from '@wails/go/main/App';
import type { WorkflowDef } from '@/types/workflow';

// 创建工作流的入参
export interface CreateWorkflowInput {
  name: string;
  description?: string;
}

// 创建工作流的返回值
export interface CreateWorkflowResponse {
  id: string;
}

// 仅用于列表展示的精简结构
export type WorkflowSummary = Pick<WorkflowDef, 'id' | 'name' | 'description'>;

const KEY = {
  list: ['workflows'] as const,
  detail: (id: string) => ['workflows', id] as const,
};

export function useWorkflows(): UseQueryResult<WorkflowSummary[]> {
  return useQuery({
    queryKey: KEY.list,
    queryFn: () => ListWorkflows() as Promise<WorkflowSummary[]>,
  });
}

export function useWorkflow(
  id: string | undefined,
): UseQueryResult<WorkflowDef> {
  return useQuery({
    queryKey: id ? KEY.detail(id) : ['workflows', 'undefined'],
    queryFn: () => GetWorkflow(id!) as Promise<WorkflowDef>,
    enabled: !!id,
  });
}

export function useCreateWorkflow(): UseMutationResult<
  CreateWorkflowResponse,
  Error,
  CreateWorkflowInput
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input) => {
      const id = await CreateWorkflow(input.name, input.description ?? '');
      return { id };
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: KEY.list });
    },
  });
}

export function useUpdateWorkflow(): UseMutationResult<
  void,
  Error,
  WorkflowDef
> {
  const qc = useQueryClient();
  return useMutation({
    // Wails 绑定接收 Go 结构体对应的 JS 对象，自动序列化
    mutationFn: (workflow) => UpdateWorkflow(workflow as never),
    onSuccess: (_, workflow) => {
      qc.invalidateQueries({ queryKey: KEY.detail(workflow.id) });
      qc.invalidateQueries({ queryKey: KEY.list });
    },
  });
}

export function useDeleteWorkflow(): UseMutationResult<void, Error, string> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id) => DeleteWorkflow(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: KEY.list });
    },
  });
}
