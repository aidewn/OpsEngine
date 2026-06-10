// AboutDialog 组件：展示应用基础信息。
import { Dialog } from '@/components/ui/Dialog';
import { Button } from '@/components/ui/Button';

interface AboutDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function AboutDialog({ open, onOpenChange }: AboutDialogProps) {
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="关于 OpsEngine"
      size="sm"
      footer={
        <Button type="button" variant="secondary" onClick={() => onOpenChange(false)}>
          关闭
        </Button>
      }
    >
      <div className="space-y-3 text-sm text-ops-secondary">
        <p className="text-ops-primary">OpsEngine</p>
        <p>面向运维工作流、巡检和报告生成的本地桌面工具。</p>
        <p className="font-mono text-2xs text-ops-tertiary">Frontend redesign phase 3</p>
      </div>
    </Dialog>
  );
}
