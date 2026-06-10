// 设置菜单：标题栏右侧的低频功能入口。
import { useState } from 'react';
import type { ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { Quit } from '@wails/runtime';
import { Info, LogOut, RefreshCcw, ServerCog, Settings } from 'lucide-react';
import { AboutDialog } from '@/components/AboutDialog';
import { Dropdown, DropdownItem, DropdownSeparator } from '@/components/ui/Dropdown';
import { AISettingsDialog } from '@/features/settings/AISettingsDialog';
import { hasWailsRuntime } from '@/lib/wailsRuntime';

export function SettingsMenu() {
  const navigate = useNavigate();
  const [aiSettingsOpen, setAISettingsOpen] = useState(false);
  const [aboutOpen, setAboutOpen] = useState(false);

  // Radix Dropdown 与 Dialog 同时切 focus 容易造成桌面端交互卡住。
  // 菜单项先完成关闭，再在下一帧打开 Dialog。
  function openAfterMenuClose(openDialog: () => void) {
    window.requestAnimationFrame(openDialog);
  }

  return (
    <>
      <Dropdown
        trigger={
          <button type="button" className="titlebar-no-drag titlebar-icon-button" aria-label="设置" title="设置">
            <Settings size={14} />
          </button>
        }
      >
        <DropdownItem onSelect={() => openAfterMenuClose(() => setAISettingsOpen(true))}>
          <MenuItem icon={<Settings size={14} />} label="AI 设置" />
        </DropdownItem>
        <DropdownItem onSelect={() => navigate('/settings/environments')}>
          <MenuItem icon={<ServerCog size={14} />} label="环境配置" />
        </DropdownItem>
        <DropdownSeparator />
        <DropdownItem onSelect={() => openAfterMenuClose(() => setAboutOpen(true))}>
          <MenuItem icon={<Info size={14} />} label="关于 OpsEngine" />
        </DropdownItem>
        <DropdownItem disabled>
          <MenuItem icon={<RefreshCcw size={14} />} label="检查更新" />
        </DropdownItem>
        <DropdownSeparator />
        <DropdownItem
          onSelect={() => {
            if (hasWailsRuntime()) Quit();
          }}
        >
          <MenuItem icon={<LogOut size={14} />} label="退出" />
        </DropdownItem>
      </Dropdown>
      <AISettingsDialog open={aiSettingsOpen} onOpenChange={setAISettingsOpen} />
      <AboutDialog open={aboutOpen} onOpenChange={setAboutOpen} />
    </>
  );
}

function MenuItem({ icon, label }: { icon: ReactNode; label: string }) {
  return (
    <span className="flex items-center gap-2">
      <span className="text-ops-secondary">{icon}</span>
      <span>{label}</span>
    </span>
  );
}
