// 节点类型 API

import { useQuery, type UseQueryResult } from '@tanstack/react-query';
import { api } from './client';
import type { NodeTypeDef } from '@/types/nodeType';

export function useNodeTypes(): UseQueryResult<NodeTypeDef[]> {
  return useQuery({
    queryKey: ['node-types'],
    queryFn: () => api.get<NodeTypeDef[]>('/api/node-types'),
    // 节点类型在运行时不会变化，缓存久一点
    staleTime: 1000 * 60 * 30,
  });
}
