// 集合相关 API hooks（TanStack Query + Wails 绑定）

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';
import {
  ListAssembles,
  GetAssemble,
  CreateAssemble,
  UpdateAssemble,
  DeleteAssemble,
} from '@wails/go/main/App';
import type { AssembleDef } from '@/types/assemble';

// 创建集合的入参
export interface CreateAssembleInput {
  name: string;
  description?: string;
}

// 仅用于列表展示的精简结构
export type AssembleSummary = Pick<AssembleDef, 'id' | 'name' | 'description'>;

const KEY = {
  list: ['assembles'] as const,
  detail: (id: string) => ['assembles', id] as const,
};

// 集合的 CRUD 会影响动态生成的节点类型，需要同步 invalidate
const NODE_TYPES_KEY = ['node-types'];

export function useAssembles(): UseQueryResult<AssembleSummary[]> {
  return useQuery({
    queryKey: KEY.list,
    queryFn: () => ListAssembles() as Promise<AssembleSummary[]>,
  });
}

export function useAssemble(
  id: string | undefined,
): UseQueryResult<AssembleDef> {
  return useQuery({
    queryKey: id ? KEY.detail(id) : ['assembles', 'undefined'],
    queryFn: () => GetAssemble(id!) as Promise<AssembleDef>,
    enabled: !!id,
  });
}

export function useCreateAssemble(): UseMutationResult<
  { id: string },
  Error,
  CreateAssembleInput
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input) => {
      const id = await CreateAssemble(input.name, input.description ?? '');
      return { id };
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: KEY.list });
      qc.invalidateQueries({ queryKey: NODE_TYPES_KEY });
    },
  });
}

export function useUpdateAssemble(): UseMutationResult<
  void,
  Error,
  AssembleDef
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (assemble) => UpdateAssemble(assemble as never),
    onMutate: async (assemble) => {
      const detailKey = KEY.detail(assemble.id);
      await qc.cancelQueries({ queryKey: detailKey });
      const prev = qc.getQueryData<AssembleDef>(detailKey);
      qc.setQueryData(detailKey, assemble);
      return { prev };
    },
    onError: (_err, assemble, ctx) => {
      if (ctx?.prev !== undefined) {
        qc.setQueryData(KEY.detail(assemble.id), ctx.prev);
      }
    },
    onSuccess: (_data, assemble) => {
      qc.setQueryData(KEY.detail(assemble.id), assemble);
      qc.invalidateQueries({ queryKey: KEY.list });
      qc.invalidateQueries({ queryKey: NODE_TYPES_KEY });
    },
  });
}

export function useDeleteAssemble(): UseMutationResult<void, Error, string> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id) => DeleteAssemble(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: KEY.list });
      qc.invalidateQueries({ queryKey: NODE_TYPES_KEY });
    },
  });
}
