// OpsDoc API hooks。

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';
import {
  ListOpsDocs,
  GetOpsDoc,
  DeleteOpsDoc,
  GenerateInspectionReport,
} from '@wails/go/main/App';
import type { OpsDoc, OpsDocSummary } from '@/types/opsDoc';

const KEY = {
  list: ['ops-docs'] as const,
  detail: (id: string) => ['ops-docs', id] as const,
};

// useOpsDocs 列出全部文档（按 UpdatedAt 倒序）。
export function useOpsDocs(): UseQueryResult<OpsDocSummary[]> {
  return useQuery({
    queryKey: KEY.list,
    queryFn: async () => {
      const raw = await ListOpsDocs();
      return (Array.isArray(raw) ? raw : []) as OpsDocSummary[];
    },
  });
}

// useOpsDoc 按 ID 加载完整文档（含 Markdown body）。
export function useOpsDoc(id: string | undefined): UseQueryResult<OpsDoc> {
  return useQuery({
    queryKey: id ? KEY.detail(id) : ['ops-docs', 'undef'],
    queryFn: () => GetOpsDoc(id!) as unknown as Promise<OpsDoc>,
    enabled: !!id,
  });
}

// useDeleteOpsDoc 删除文档。
export function useDeleteOpsDoc(): UseMutationResult<void, Error, string> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id) => DeleteOpsDoc(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: KEY.list });
    },
  });
}

// useGenerateInspectionReport 根据执行记录生成巡检报告，成功后返回完整 OpsDoc。
export function useGenerateInspectionReport(): UseMutationResult<OpsDoc, Error, string> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (executionID) =>
      GenerateInspectionReport(executionID) as unknown as Promise<OpsDoc>,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: KEY.list });
    },
  });
}
