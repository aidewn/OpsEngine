// 在集合编辑页向画布内部传递当前 AssembleDef
// 主要供 AssembleEndNode 根据 returns 动态渲染 input 端口

import { createContext, useContext, type ReactNode } from 'react';
import type { AssembleDef } from '@/types/assemble';

const AssembleContext = createContext<AssembleDef | null>(null);

// AssembleProvider 由 AssembleCanvasPage 包在画布外面
export function AssembleProvider({
  value,
  children,
}: {
  value: AssembleDef;
  children: ReactNode;
}) {
  return (
    <AssembleContext.Provider value={value}>{children}</AssembleContext.Provider>
  );
}

// useCurrentAssemble 不在 Provider 内（工作流编辑页/执行详情页）返回 null
export function useCurrentAssemble(): AssembleDef | null {
  return useContext(AssembleContext);
}
