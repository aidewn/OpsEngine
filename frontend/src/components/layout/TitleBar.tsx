// 自绘标题栏：承载窗口拖拽、历史导航、命令面板、设置入口和窗口控制。
import { Command, PanelLeft, Pin } from 'lucide-react';
import { COMMAND_PALETTE_EVENT } from './CommandPalette';
import { NavHistoryButtons } from './NavHistoryButtons';
import { SettingsMenu } from './SettingsMenu';
import { TitleBarCenter } from './TitleBarCenter';
import { WindowControls } from './WindowControls';

export function TitleBar() {
  return (
    <header className="flex h-8 shrink-0 items-center border-b border-ops-border-subtle bg-ops-titlebar text-ops-primary">
      <div className="titlebar-no-drag flex w-40 items-center gap-1 px-2">
        <button type="button" className="titlebar-icon-button" aria-label="侧栏" title="侧栏" disabled>
          <PanelLeft size={14} />
        </button>
        <button type="button" className="titlebar-icon-button" aria-label="固定侧栏" title="固定侧栏" disabled>
          <Pin size={14} />
        </button>
        <button
          type="button"
          className="titlebar-icon-button"
          aria-label="命令面板"
          title="命令面板"
          onClick={() => window.dispatchEvent(new Event(COMMAND_PALETTE_EVENT))}
        >
          <Command size={14} />
        </button>
        <NavHistoryButtons />
      </div>
      <TitleBarCenter />
      <div className="flex w-40 items-center justify-end gap-1 pr-1">
        <SettingsMenu />
        <WindowControls />
      </div>
    </header>
  );
}
