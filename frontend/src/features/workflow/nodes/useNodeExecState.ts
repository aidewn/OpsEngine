// 节点在当前活动 execution 中的状态
// 通过 URL pathname 判断 + 当前 framePath（通过 React Context 注入）

import { createContext, useContext } from 'react';
import { useLocation } from 'react-router-dom';
import { frameAt, useExecution } from '@/features/execution/ExecutionStore';
import { activeTabFromPath } from '@/features/tabs/TabsContext';
import type { NodeState } from '@/types/execution';

// FramePathContext 由 ExecutionDetailPage 注入当前查看的 frame 路径
// 工作流编辑页 / 集合编辑页不注入 = 默认 []
export const FramePathContext = createContext<string[]>([]);

export function useNodeExecState(nodeID: string): NodeState | undefined {
  const location = useLocation();
  const active = activeTabFromPath(location.pathname);
  const execID = active?.kind === 'execution' ? active.id : undefined;
  const exec = useExecution(execID);
  const framePath = useContext(FramePathContext);
  const frame = frameAt(exec?.rootFrame, framePath);
  return frame?.node_states[nodeID];
}
