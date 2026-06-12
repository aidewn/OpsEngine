// ViewActions：把"卡片要触发的会话级动作"通过 context 注入，
// 避免回调层层穿透 MessageList → Bubble → ViewRenderer。
// AIAssistantPanel 在根部 provide；卡片组件按需 consume。
import { createContext, useContext } from 'react';

export interface ViewActions {
  // 打开执行详情页（执行卡「查看执行」）
  openExecution?: (executionID: string) => void;
  // 复用当前会话重发一条消息（卡片「重新采集」）
  resend?: (message: string) => void;
}

const ViewActionsContext = createContext<ViewActions>({});

export const ViewActionsProvider = ViewActionsContext.Provider;

export function useViewActions(): ViewActions {
  return useContext(ViewActionsContext);
}
