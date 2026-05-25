// 节点剪贴板：React Context 内存存储
// 仅同一会话同一编辑器内有效；切换 tab 时清空（由 page 主动调 clear）

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import type { EdgeConfig, NodeInstance } from '@/types/workflow';

// 剪贴板数据
export interface ClipboardData {
  sourceEditorKey: string; // "workflow:<id>" 或 "assemble:<id>"，跨 tab 时不匹配 → 清空
  nodes: NodeInstance[];
  edges: EdgeConfig[];
}

interface ContextValue {
  data: ClipboardData | null;
  set: (data: ClipboardData) => void;
  clear: () => void;
}

const ClipboardContext = createContext<ContextValue | null>(null);

export function ClipboardProvider({ children }: { children: ReactNode }) {
  const [data, setData] = useState<ClipboardData | null>(null);
  const set = useCallback((d: ClipboardData) => setData(d), []);
  const clear = useCallback(() => setData(null), []);
  // Provider value 稳定化：避免每次 render 都新建对象触发下游 useEffect 重订阅
  const value = useMemo(() => ({ data, set, clear }), [data, set, clear]);
  return (
    <ClipboardContext.Provider value={value}>
      {children}
    </ClipboardContext.Provider>
  );
}

export function useClipboard(): ContextValue {
  const ctx = useContext(ClipboardContext);
  if (!ctx) throw new Error('useClipboard 必须在 ClipboardProvider 内使用');
  return ctx;
}
