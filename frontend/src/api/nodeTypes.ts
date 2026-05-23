// 节点类型 API（Wails 绑定）

import { useQuery, type UseQueryResult } from '@tanstack/react-query';
import { GetNodeTypes } from '@wails/go/main/App';
import type { NodeTypeDef } from '@/types/nodeType';

export function useNodeTypes(): UseQueryResult<NodeTypeDef[]> {
  return useQuery({
    queryKey: ['node-types'],
    queryFn: () => GetNodeTypes() as Promise<NodeTypeDef[]>,
    // 节点类型在运行时不会变化，缓存久一点
    staleTime: 1000 * 60 * 30,
  });
}
