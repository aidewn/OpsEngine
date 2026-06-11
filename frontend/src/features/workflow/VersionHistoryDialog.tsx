// 工作流历史版本对话框：列出最近 20 份快照，支持一键恢复。
// 恢复操作本身也会产生快照，因此误恢复可再次恢复回来。

import { useState } from 'react';
import { toast } from 'sonner';
import { Dialog } from '@/components/ui/Dialog';
import { Button } from '@/components/ui/Button';
import {
  useWorkflowVersions,
  useRestoreWorkflowVersion,
} from '@/api/workflows';

interface VersionHistoryDialogProps {
  workflowID: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function VersionHistoryDialog({
  workflowID,
  open,
  onOpenChange,
}: VersionHistoryDialogProps) {
  const { data: versions, isLoading } = useWorkflowVersions(workflowID, open);
  const restore = useRestoreWorkflowVersion();
  // 二次确认：第一次点「恢复」记录目标，再点一次才执行
  const [confirming, setConfirming] = useState<string | null>(null);

  async function handleRestore(version: string) {
    if (confirming !== version) {
      setConfirming(version);
      return;
    }
    try {
      await restore.mutateAsync({ id: workflowID, version });
      toast.success('已恢复历史版本（恢复前的版本也已存入历史）');
      setConfirming(null);
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '恢复失败');
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        setConfirming(null);
        onOpenChange(o);
      }}
      title="历史版本"
      description="每次保存（含 AI 修改）自动快照，保留最近 20 份。"
    >
      <div className="max-h-80 overflow-y-auto">
        {isLoading && (
          <div className="py-6 text-center text-sm text-slate-500">加载中...</div>
        )}
        {!isLoading && (!versions || versions.length === 0) && (
          <div className="py-6 text-center text-sm text-slate-500">
            暂无历史版本（首次修改后产生）
          </div>
        )}
        {versions && versions.length > 0 && (
          <ul className="divide-y divide-ops-border-subtle">
            {versions.map((v) => (
              <li
                key={v.version}
                className="flex items-center justify-between py-2"
              >
                <span className="font-mono text-xs text-slate-600">
                  {new Date(v.saved_at).toLocaleString()}
                </span>
                <Button
                  size="sm"
                  variant={confirming === v.version ? 'danger' : 'secondary'}
                  disabled={restore.isPending}
                  onClick={() => void handleRestore(v.version)}
                >
                  {confirming === v.version ? '确认覆盖当前？' : '恢复'}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Dialog>
  );
}
