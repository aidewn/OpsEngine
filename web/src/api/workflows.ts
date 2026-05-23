// 工作流相关 API hooks（TanStack Query 封装）

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';
import { api } from './client';
import type { WorkflowDef } from '@/types/workflow';

// 创建工作流的入参（后端会生成 id 并返回）
export interface CreateWorkflowInput {
  name: string;
  description?: string;
}

export interface CreateWorkflowResponse {
  id: string;
}

// 仅用于列表展示的精简结构（后端可以返回完整 WorkflowDef，前端只取需要的字段）
export type WorkflowSummary = Pick<WorkflowDef, 'id' | 'name' | 'description'>;

const KEY = {
  list: ['workflows'] as const,
  detail: (id: string) => ['workflows', id] as const,
};

export function useWorkflows(): UseQueryResult<WorkflowSummary[]> {
  return useQuery({
    queryKey: KEY.list,
    queryFn: () => api.get<WorkflowSummary[]>('/api/workflows'),
  });
}

export function useWorkflow(
  id: string | undefined,
): UseQueryResult<WorkflowDef> {
  return useQuery({
    queryKey: id ? KEY.detail(id) : ['workflows', 'undefined'],
    queryFn: () => api.get<WorkflowDef>(`/api/workflows/${id}`),
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
    mutationFn: (input) =>
      api.post<CreateWorkflowResponse>('/api/workflows', input),
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
    mutationFn: (workflow) =>
      api.put<void>(`/api/workflows/${workflow.id}`, workflow),
    onSuccess: (_, workflow) => {
      qc.invalidateQueries({ queryKey: KEY.detail(workflow.id) });
      qc.invalidateQueries({ queryKey: KEY.list });
    },
  });
}

export function useDeleteWorkflow(): UseMutationResult<void, Error, string> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id) => api.delete<void>(`/api/workflows/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: KEY.list });
    },
  });
}
