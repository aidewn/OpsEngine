// 「确认恢复」按钮：把「正常（有异常历史）」监控项确认归档为纯正常。
// 可放在概览行内或监控项详情；点击阻止冒泡，避免触发所在行的跳转。
import { useAcknowledgePanelHistory } from '@/api/monitor';
import { Button } from '@/components/ui/Button';
import { errorMessage } from '@/lib/error';
import { toast } from '@/lib/toast';

export function AcknowledgeButton({ panelID }: { panelID: string }) {
  const ack = useAcknowledgePanelHistory();

  function handleClick(e: React.MouseEvent) {
    e.stopPropagation();
    ack.mutate(panelID, {
      onSuccess: () => toast.success('已确认恢复'),
      onError: (err) => toast.error(errorMessage(err, '确认失败')),
    });
  }

  return (
    <Button size="sm" variant="secondary" disabled={ack.isPending} onClick={handleClick}>
      {ack.isPending ? '确认中...' : '确认恢复'}
    </Button>
  );
}
