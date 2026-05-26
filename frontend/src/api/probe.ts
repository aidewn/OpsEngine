// 编辑态探测 API hook
// 与 useTestEnvConfig 同结构：不缓存，调用方自行处理结果与错误

import { useMutation, type UseMutationResult } from '@tanstack/react-query';
import { ProbeEnvNode } from '@wails/go/main/App';
import type {
  ProbeEnvNodeRequest,
  ProbeEnvNodeResult,
} from '@/types/probe';

export function useProbeEnvNode(): UseMutationResult<
  ProbeEnvNodeResult,
  Error,
  ProbeEnvNodeRequest
> {
  return useMutation({
    mutationFn: (req) =>
      ProbeEnvNode(req as never) as Promise<ProbeEnvNodeResult>,
  });
}
