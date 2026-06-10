// AISettingsDialog 组件：从标题栏设置菜单打开 AI 设置。
import { Dialog } from '@/components/ui/Dialog';
import { SettingsPage } from './SettingsPage';

interface AISettingsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function AISettingsDialog({ open, onOpenChange }: AISettingsDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title="AI 设置" size="lg">
      <SettingsPage embedded />
    </Dialog>
  );
}
