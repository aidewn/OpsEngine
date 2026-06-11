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
  ListWorkflowVersions,
  RestoreWorkflowVersion,
} from '@wails/go/main/App';
import type { WorkflowDef } from '@/types/workflow';

// 历史版本元信息（对应后端 store.VersionInfo）
export interface WorkflowVersion {
  version: string;
  saved_at: string;
}

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
  versions: (id: string) => ['workflows', id, 'versions'] as const,
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
    // 乐观写入缓存，避免保存后 invalidate 重拉期间用旧 workflow 覆盖画布/ConfigForm
    onMutate: async (workflow) => {
      const detailKey = KEY.detail(workflow.id);
      await qc.cancelQueries({ queryKey: detailKey });
      const prev = qc.getQueryData<WorkflowDef>(detailKey);
      qc.setQueryData(detailKey, workflow);
      return { prev };
    },
    onError: (_err, workflow, ctx) => {
      if (ctx?.prev !== undefined) {
        qc.setQueryData(KEY.detail(workflow.id), ctx.prev);
      }
    },
    onSuccess: (_data, workflow) => {
      qc.setQueryData(KEY.detail(workflow.id), workflow);
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

// useWorkflowVersions 列出工作流历史版本（最新在前）
export function useWorkflowVersions(
  id: string | undefined,
  enabled = true,
): UseQueryResult<WorkflowVersion[]> {
  return useQuery({
    queryKey: id ? KEY.versions(id) : ['workflows', 'undefined', 'versions'],
    queryFn: async () => {
      const raw = await ListWorkflowVersions(id!);
      return (Array.isArray(raw) ? raw : []) as WorkflowVersion[];
    },
    enabled: !!id && enabled,
  });
}

// useRestoreWorkflowVersion 恢复历史版本为当前版本，成功后刷新详情与版本列表
export function useRestoreWorkflowVersion(): UseMutationResult<
  WorkflowDef,
  Error,
  { id: string; version: string }
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, version }) =>
      RestoreWorkflowVersion(id, version) as Promise<WorkflowDef>,
    onSuccess: (restored, { id }) => {
      qc.setQueryData(KEY.detail(id), restored);
      qc.invalidateQueries({ queryKey: KEY.versions(id) });
      qc.invalidateQueries({ queryKey: KEY.list });
    },
  });
}
