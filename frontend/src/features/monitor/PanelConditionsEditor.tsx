// 监控项「判断条件」编辑器（Monitor Flow）：增删改阈值条件，保存即整体更新监控项。
// 每条条件从某类采集结果取一个数值字段，与阈值按运算符比较；任一命中即异常。
// target 留空表示匹配该 kind 的任一采集结果（多数监控项每种数据只采一处）。
import { useState } from 'react';
import { useUpdateMonitorPanel } from '@/api/monitor';
import { errorMessage } from '@/lib/error';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Label } from '@/components/ui/Label';
import { Select } from '@/components/ui/Select';
import type { MonitorCondition, MonitorPanel } from '@/types/monitor';
import { MONITOR_KINDS, kindDef } from './dataKinds';

const OPS: MonitorCondition['op'][] = ['>', '>=', '<', '<=', '==', '!='];
const SEVERITIES: MonitorCondition['severity'][] = ['warning', 'critical'];

export function PanelConditionsEditor({ panel }: { panel: MonitorPanel }) {
  const update = useUpdateMonitorPanel();
  const [conditions, setConditions] = useState<MonitorCondition[]>(panel.conditions ?? []);
  const [abnormalThreshold, setAbnormalThreshold] = useState(panel.abnormal_threshold || 1);
  const [recoveryThreshold, setRecoveryThreshold] = useState(panel.recovery_threshold || 1);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const busy = update.isPending;

  function patch(index: number, p: Partial<MonitorCondition>) {
    setSaved(false);
    setConditions((prev) => prev.map((c, i) => (i === index ? { ...c, ...p } : c)));
  }

  // 切换 kind 时把字段重置为该类型的首个可选字段，避免残留无效字段。
  function changeKind(index: number, kind: string) {
    const field = kindDef(kind)?.conditionFields?.[0]?.value ?? '';
    patch(index, { kind, field });
  }

  function addCondition() {
    setSaved(false);
    const kind = MONITOR_KINDS[0]?.kind ?? 'host.basic';
    setConditions((prev) => [
      ...prev,
      {
        kind,
        target_id: '',
        field: kindDef(kind)?.conditionFields?.[0]?.value ?? '',
        op: '>',
        value: 0,
        severity: 'warning',
      },
    ]);
  }

  function removeCondition(index: number) {
    setSaved(false);
    setConditions((prev) => prev.filter((_, i) => i !== index));
  }

  async function handleSave() {
    setError(null);
    try {
      await update.mutateAsync({
        ...panel,
        conditions,
        abnormal_threshold: Math.max(1, abnormalThreshold),
        recovery_threshold: Math.max(1, recoveryThreshold),
      });
      setSaved(true);
    } catch (err) {
      setError(errorMessage(err, '保存失败'));
    }
  }

  function changeThreshold(setter: (n: number) => void, value: number) {
    setSaved(false);
    setter(value);
  }

  return (
    <div className="rounded-lg border border-ops-border-subtle bg-ops-surface p-4">
      <div className="flex items-center justify-between">
        <div className="text-sm font-medium text-ops-primary">判断条件</div>
        <Button size="sm" variant="secondary" onClick={addCondition} disabled={busy}>
          添加条件
        </Button>
      </div>

      <div className="mt-3 space-y-3">
        {conditions.length === 0 ? (
          <div className="rounded-md border border-dashed border-ops-border-subtle px-3 py-4 text-xs text-ops-tertiary">
            还没有判断条件。无条件时监控项始终视为正常。
          </div>
        ) : (
          conditions.map((cond, index) => {
            const def = kindDef(cond.kind);
            return (
              <div
                key={index}
                className="space-y-3 rounded-md border border-ops-border-subtle bg-ops-elevated p-3"
              >
                <div className="grid gap-3 md:grid-cols-2">
                  <div className="space-y-1">
                    <Label>数据类型</Label>
                    <Select
                      value={cond.kind}
                      onChange={(e) => changeKind(index, e.target.value)}
                      disabled={busy}
                    >
                      {MONITOR_KINDS.map((k) => (
                        <option key={k.kind} value={k.kind}>
                          {k.label}
                        </option>
                      ))}
                    </Select>
                  </div>
                  <div className="space-y-1">
                    <Label>字段</Label>
                    {def?.conditionFields && def.conditionFields.length > 0 ? (
                      <Select
                        value={cond.field}
                        onChange={(e) => patch(index, { field: e.target.value })}
                        disabled={busy}
                      >
                        {def.conditionFields.map((f) => (
                          <option key={f.value} value={f.value}>
                            {f.label}
                          </option>
                        ))}
                      </Select>
                    ) : (
                      <Input
                        value={cond.field}
                        placeholder="字段路径，如 status_code"
                        onChange={(e) => patch(index, { field: e.target.value })}
                        disabled={busy}
                      />
                    )}
                  </div>
                </div>

                <div className="grid gap-3 md:grid-cols-3">
                  <div className="space-y-1">
                    <Label>运算符</Label>
                    <Select
                      value={cond.op}
                      onChange={(e) => patch(index, { op: e.target.value as MonitorCondition['op'] })}
                      disabled={busy}
                    >
                      {OPS.map((op) => (
                        <option key={op} value={op}>
                          {op}
                        </option>
                      ))}
                    </Select>
                  </div>
                  <div className="space-y-1">
                    <Label>阈值</Label>
                    <Input
                      type="number"
                      value={String(cond.value)}
                      onChange={(e) => patch(index, { value: Number(e.target.value) })}
                      disabled={busy}
                    />
                  </div>
                  <div className="space-y-1">
                    <Label>级别</Label>
                    <Select
                      value={cond.severity}
                      onChange={(e) =>
                        patch(index, { severity: e.target.value as MonitorCondition['severity'] })
                      }
                      disabled={busy}
                    >
                      {SEVERITIES.map((s) => (
                        <option key={s} value={s}>
                          {s === 'warning' ? '警告' : '严重'}
                        </option>
                      ))}
                    </Select>
                  </div>
                </div>

                <div className="flex justify-end">
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => removeCondition(index)}
                    disabled={busy}
                  >
                    移除
                  </Button>
                </div>
              </div>
            );
          })
        )}
      </div>

      <div className="mt-4 flex flex-wrap items-center gap-2 border-t border-ops-border-subtle pt-3">
        <span className="text-xs text-ops-tertiary" title="连续异常达到该次数才告警，抑制瞬时抖动">
          异常防抖
        </span>
        <Input
          type="number"
          className="h-7 w-16"
          min={1}
          value={String(abnormalThreshold)}
          disabled={busy}
          onChange={(e) => changeThreshold(setAbnormalThreshold, Number(e.target.value))}
        />
        <span className="text-xs text-ops-tertiary" title="连续正常达到该次数才解除异常">
          恢复防抖
        </span>
        <Input
          type="number"
          className="h-7 w-16"
          min={1}
          value={String(recoveryThreshold)}
          disabled={busy}
          onChange={(e) => changeThreshold(setRecoveryThreshold, Number(e.target.value))}
        />
        <span className="text-xs text-ops-tertiary">次（1 = 即时）</span>
      </div>

      {error ? (
        <div className="mt-3 rounded border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700">
          {error}
        </div>
      ) : null}

      <div className="mt-4 flex items-center gap-3">
        <Button size="sm" onClick={handleSave} disabled={busy}>
          {busy ? '保存中...' : '保存判断条件'}
        </Button>
        {saved ? <span className="text-xs text-ops-success">已保存</span> : null}
      </div>
    </div>
  );
}
