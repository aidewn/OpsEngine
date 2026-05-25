// 端口右键上下文菜单：当前只承载「提升为变量」一项
// 渲染为 fixed 浮层，点击外部 / Esc 自动关闭

import { useEffect, useRef } from 'react';

export interface PortContextMenuState {
  x: number;
  y: number;
  nodeId: string;
  portId: string;
  portLabel: string;
  portType: string;
}

interface Props {
  state: PortContextMenuState;
  onClose: () => void;
  onPromote: () => void;
}

export function PortContextMenu({ state, onClose, onPromote }: Props) {
  const ref = useRef<HTMLDivElement>(null);

  // 点击菜单外部或按 Esc 关闭
  useEffect(() => {
    function onMouseDown(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onClose();
      }
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose();
    }
    window.addEventListener('mousedown', onMouseDown);
    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('mousedown', onMouseDown);
      window.removeEventListener('keydown', onKey);
    };
  }, [onClose]);

  return (
    <div
      ref={ref}
      className="fixed z-50 min-w-[180px] rounded-md border border-slate-200 bg-white py-1 text-xs shadow-lg"
      style={{ left: state.x, top: state.y }}
    >
      <div className="border-b border-slate-100 px-3 py-1 text-[11px] text-slate-500">
        {state.portLabel}
        <span className="ml-1 text-slate-400">({state.portType})</span>
      </div>
      <button
        type="button"
        className="block w-full px-3 py-1.5 text-left hover:bg-slate-100"
        onClick={() => {
          onPromote();
          onClose();
        }}
      >
        提升为变量
      </button>
    </div>
  );
}
