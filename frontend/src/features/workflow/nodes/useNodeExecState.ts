// 节点在当前活动 execution 中的状态
// 通过 URL pathname 判断：仅在 /executions/<id> 路径下生效
// 工作流编辑页 / 集合编辑页返回 undefined（不显示执行状态）

import { useLocation } from 'react-router-dom';
import { useExecution } from '@/features/execution/ExecutionStore';
import { activeTabFromPath } from '@/features/tabs/TabsContext';
import type { NodeState } from '@/types/execution';

export function useNodeExecState(nodeID: string): NodeState | undefined {
  const location = useLocation();
  const active = activeTabFromPath(location.pathname);
  const execID = active?.kind === 'execution' ? active.id : undefined;
  const exec = useExecution(execID);
  return exec?.nodeStates[nodeID];
}
