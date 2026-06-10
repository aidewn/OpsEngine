// 窗口控制按钮：封装 Wails 的最小化、最大化和退出能力。
import { useEffect, useState } from 'react';
import { Environment, Quit, WindowMinimise, WindowToggleMaximise } from '@wails/runtime';
import { Minus, Square, X } from 'lucide-react';
import { hasWailsRuntime } from '@/lib/wailsRuntime';

export function WindowControls() {
  const [isMacOS, setIsMacOS] = useState(false);

  useEffect(() => {
    // macOS 使用系统 traffic lights，避免和自绘按钮重复。
    if (!hasWailsRuntime()) {
      setIsMacOS(navigator.platform.toLowerCase().includes('mac'));
      return;
    }

    Environment()
      .then((environment) => setIsMacOS(environment.platform === 'darwin'))
      .catch(() => setIsMacOS(false));
  }, []);

  if (isMacOS) return null;

  return (
    <div className="titlebar-no-drag flex items-center">
      <button
        type="button"
        className="titlebar-window-button"
        aria-label="最小化"
        title="最小化"
        onClick={() => WindowMinimise()}
      >
        <Minus size={14} />
      </button>
      <button
        type="button"
        className="titlebar-window-button"
        aria-label="最大化"
        title="最大化"
        onClick={() => WindowToggleMaximise()}
      >
        <Square size={12} />
      </button>
      <button
        type="button"
        className="titlebar-window-button titlebar-close-button"
        aria-label="关闭"
        title="关闭"
        onClick={() => Quit()}
      >
        <X size={14} />
      </button>
    </div>
  );
}
