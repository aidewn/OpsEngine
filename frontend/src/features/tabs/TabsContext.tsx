// 顶部 tab 状态管理：维护打开的工作流/集合列表
// 内存存储，会话结束即清空（设计决策：tab 状态不持久化）

import {
  createContext,
  useCallback,
  useContext,
  useReducer,
  type ReactNode,
} from 'react';

export type TabKind = 'workflow' | 'assemble';

// 单个 tab 项
export interface TabItem {
  kind: TabKind;
  id: string;
  name: string; // 直接 URL 访问时先用 id 占位，doc 加载后通过 openTab/renameTab 更新
}

interface TabsState {
  tabs: TabItem[];
}

type Action =
  | { type: 'open'; tab: TabItem }
  | { type: 'close'; kind: TabKind; id: string }
  | { type: 'rename'; kind: TabKind; id: string; name: string };

// reducer：open 行为为 upsert（已存在则更新 name，避免占位名残留）
function reducer(state: TabsState, action: Action): TabsState {
  switch (action.type) {
    case 'open': {
      const idx = state.tabs.findIndex(
        (t) => t.kind === action.tab.kind && t.id === action.tab.id,
      );
      if (idx >= 0) {
        const existing = state.tabs[idx];
        if (!existing || existing.name === action.tab.name) return state;
        const next = [...state.tabs];
        next[idx] = { ...existing, name: action.tab.name };
        return { tabs: next };
      }
      return { tabs: [...state.tabs, action.tab] };
    }
    case 'close':
      return {
        tabs: state.tabs.filter(
          (t) => !(t.kind === action.kind && t.id === action.id),
        ),
      };
    case 'rename':
      return {
        tabs: state.tabs.map((t) =>
          t.kind === action.kind && t.id === action.id
            ? { ...t, name: action.name }
            : t,
        ),
      };
    default:
      return state;
  }
}

interface TabsContextValue {
  tabs: TabItem[];
  openTab: (tab: TabItem) => void;
  closeTab: (kind: TabKind, id: string) => void;
  renameTab: (kind: TabKind, id: string, name: string) => void;
}

const TabsContext = createContext<TabsContextValue | null>(null);

export function TabsProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, { tabs: [] });

  // dispatch 稳定，回调用 useCallback 包一层方便外部 useEffect 依赖
  const openTab = useCallback(
    (tab: TabItem) => dispatch({ type: 'open', tab }),
    [],
  );
  const closeTab = useCallback(
    (kind: TabKind, id: string) => dispatch({ type: 'close', kind, id }),
    [],
  );
  const renameTab = useCallback(
    (kind: TabKind, id: string, name: string) =>
      dispatch({ type: 'rename', kind, id, name }),
    [],
  );

  return (
    <TabsContext.Provider
      value={{ tabs: state.tabs, openTab, closeTab, renameTab }}
    >
      {children}
    </TabsContext.Provider>
  );
}

export function useTabs(): TabsContextValue {
  const ctx = useContext(TabsContext);
  if (!ctx) throw new Error('useTabs 必须在 TabsProvider 内使用');
  return ctx;
}

// 由 tab 推出路由路径
export function routeFor(tab: TabItem | { kind: TabKind; id: string }): string {
  return tab.kind === 'workflow'
    ? `/workflows/${tab.id}`
    : `/assembles/${tab.id}`;
}

// 由当前 URL pathname 推出激活 tab 的 kind+id（找不到返回 null，例如在首页）
export function activeTabFromPath(
  pathname: string,
): { kind: TabKind; id: string } | null {
  const wf = pathname.match(/^\/workflows\/([^/]+)/);
  if (wf && wf[1]) return { kind: 'workflow', id: wf[1] };
  const asm = pathname.match(/^\/assembles\/([^/]+)/);
  if (asm && asm[1]) return { kind: 'assemble', id: asm[1] };
  return null;
}
