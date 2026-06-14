// 环境监控调度配置：开关「自动监控」并设置采集间隔（秒）。
// 开启后由后台调度器按间隔自动采集判断。防抖阈值配在各监控项上（见判断条件编辑器）。
import { useEffect, useState } from 'react';
import { useMonitorConfig, useSetMonitorConfig } from '@/api/monitor';
import { Input } from '@/components/ui/Input';
import { errorMessage } from '@/lib/error';
import { toast } from '@/lib/toast';

const MIN_INTERVAL = 10;

export function MonitorConfigControl({ environmentID }: { environmentID: string }) {
  const { data: config } = useMonitorConfig(environmentID);
  const setConfig = useSetMonitorConfig();
  const [interval, setIntervalValue] = useState(60);

  // 配置加载/切换环境后同步本地间隔输入。
  useEffect(() => {
    if (config) setIntervalValue(config.interval_seconds);
  }, [config]);

  if (!config) return null;

  function save(enabled: boolean, intervalSeconds: number) {
    setConfig.mutate(
      { environmentID, enabled, intervalSeconds: Math.max(MIN_INTERVAL, intervalSeconds) },
      { onError: (err) => toast.error(errorMessage(err, '保存监控配置失败')) },
    );
  }

  return (
    <div className="flex flex-wrap items-center gap-2 rounded-md border border-ops-border-subtle bg-ops-surface px-3 py-1.5">
      <label className="flex items-center gap-2 text-sm text-ops-secondary">
        <input
          type="checkbox"
          checked={config.enabled}
          disabled={setConfig.isPending}
          onChange={(e) => save(e.target.checked, interval)}
        />
        自动监控
      </label>
      <span className="text-xs text-ops-tertiary">间隔</span>
      <Input
        type="number"
        className="h-7 w-20"
        min={MIN_INTERVAL}
        value={String(interval)}
        disabled={setConfig.isPending}
        onChange={(e) => setIntervalValue(Number(e.target.value))}
        onBlur={() => {
          if (config.enabled && interval !== config.interval_seconds) save(true, interval);
        }}
      />
      <span className="text-xs text-ops-tertiary">秒</span>
      {config.enabled ? (
        <span className="text-xs text-ops-success">运行中</span>
      ) : (
        <span className="text-xs text-ops-tertiary">已停用</span>
      )}
    </div>
  );
}
