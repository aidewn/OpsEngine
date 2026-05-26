// 环境相关 API hooks（TanStack Query + Wails 绑定）
// 与 workflows.ts 对齐：list/detail 拆分 query key、乐观更新 update、test 连接为独立 mutation

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';
import {
  ListEnvironments,
  GetEnvironment,
  CreateEnvironment,
  UpdateEnvironment,
  DeleteEnvironment,
  TestEnvConfig,
} from '@wails/go/main/App';
import type { EnvironmentDef } from '@/types/environment';

// 创建环境的入参
export interface CreateEnvironmentInput {
  name: string;
  description?: string;
}

// 创建环境的返回值
export interface CreateEnvironmentResponse {
  id: string;
}

// 测试连接入参（envID + configID）
export interface TestEnvConfigInput {
  envID: string;
  configID: string;
}

const KEY = {
  list: ['environments'] as const,
  detail: (id: string) => ['environments', id] as const,
};

export function useEnvironments(): UseQueryResult<EnvironmentDef[]> {
  return useQuery({
    queryKey: KEY.list,
    queryFn: () => ListEnvironments() as Promise<EnvironmentDef[]>,
  });
}

export function useEnvironment(
  id: string | undefined,
): UseQueryResult<EnvironmentDef> {
  return useQuery({
    queryKey: id ? KEY.detail(id) : ['environments', 'undefined'],
    queryFn: () => GetEnvironment(id!) as Promise<EnvironmentDef>,
    enabled: !!id,
  });
}

export function useCreateEnvironment(): UseMutationResult<
  CreateEnvironmentResponse,
  Error,
  CreateEnvironmentInput
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input) => {
      const id = await CreateEnvironment(input.name, input.description ?? '');
      return { id };
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: KEY.list });
    },
  });
}

export function useUpdateEnvironment(): UseMutationResult<
  void,
  Error,
  EnvironmentDef
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (env) => UpdateEnvironment(env as never),
    // 乐观写入 detail 缓存：避免保存后 invalidate 重拉期间表单回退到旧值
    onMutate: async (env) => {
      const detailKey = KEY.detail(env.id);
      await qc.cancelQueries({ queryKey: detailKey });
      const prev = qc.getQueryData<EnvironmentDef>(detailKey);
      qc.setQueryData(detailKey, env);
      return { prev };
    },
    onError: (_err, env, ctx) => {
      if (ctx?.prev !== undefined) {
        qc.setQueryData(KEY.detail(env.id), ctx.prev);
      }
    },
    onSuccess: (_data, env) => {
      qc.setQueryData(KEY.detail(env.id), env);
      qc.invalidateQueries({ queryKey: KEY.list });
    },
  });
}

export function useDeleteEnvironment(): UseMutationResult<void, Error, string> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id) => DeleteEnvironment(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: KEY.list });
    },
  });
}

// 测试单条配置连通性；不缓存，调用方拿 mutation 结果自行 toast
export function useTestEnvConfig(): UseMutationResult<
  void,
  Error,
  TestEnvConfigInput
> {
  return useMutation({
    mutationFn: ({ envID, configID }) => TestEnvConfig(envID, configID),
  });
}
