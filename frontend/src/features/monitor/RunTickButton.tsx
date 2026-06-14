// 「立即检查一次」按钮：手动触发一轮监控（采集+判断+更新状态），与后台自动调度共用同一逻辑。
// 常态采集由后台调度器自动进行，此按钮仅用于即时验证/调试。
import { useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { useRunMonitorTick } from '@/api/monitor';
import { Button } from '@/components/ui/Button';
import { errorMessage } from '@/lib/error';
import { toast } from '@/lib/toast';

// 最小动效时长：检查可能极快（如 http），保证旋转动效至少可见这么久。
const MIN_SPIN_MS = 600;

export function RunTickButton({ environmentID }: { environmentID: string }) {
  const tick = useRunMonitorTick();
  const [spinning, setSpinning] = useState(false);
  const busy = spinning || tick.isPending;

  function handleRun() {
    if (busy) return;
    setSpinning(true);
    const started = Date.now();
    tick.mutate(environmentID, {
      onError: (err) => toast.error(errorMessage(err, '检查失败')),
      onSettled: () => {
        const remaining = MIN_SPIN_MS - (Date.now() - started);
        window.setTimeout(() => setSpinning(false), Math.max(0, remaining));
      },
    });
  }

  return (
    <Button size="sm" variant="secondary" onClick={handleRun} disabled={busy}>
      <RefreshCw size={14} className={busy ? 'animate-spin' : ''} />
      {busy ? '检查中...' : '立即检查'}
    </Button>
  );
}
