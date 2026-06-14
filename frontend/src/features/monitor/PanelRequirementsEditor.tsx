// 监控项「数据需求」编辑器：增删改 Requirements（target/kind/params），保存即整体更新监控项。
// kind 决定目标配置类型与参数表单（见 dataKinds.ts）；http.health 不依赖环境配置。
import { useState } from 'react';
import { useEnvironment } from '@/api/environments';
import { useUpdateMonitorPanel } from '@/api/monitor';
import { errorMessage } from '@/lib/error';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Label } from '@/components/ui/Label';
import { Select } from '@/components/ui/Select';
import type { DataRequirement, MonitorPanel } from '@/types/monitor';
import { MONITOR_KINDS, kindDef } from './dataKinds';

export function PanelRequirementsEditor({ panel }: { panel: MonitorPanel }) {
  const { data: environment } = useEnvironment(panel.environment_id);
  const update = useUpdateMonitorPanel();
  const [reqs, setReqs] = useState<DataRequirement[]>(panel.requirements ?? []);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const configs = environment?.configs ?? [];
  const busy = update.isPending;

  function patchReq(index: number, patch: Partial<DataRequirement>) {
    setSaved(false);
    setReqs((prev) => prev.map((r, i) => (i === index ? { ...r, ...patch } : r)));
  }

  // 切换 kind 时清空目标与参数，避免残留上一类型的无效字段。
  function changeKind(index: number, kind: string) {
    patchReq(index, { kind, target_id: '', params: {} });
  }

  function setParam(index: number, key: string, value: unknown) {
    setSaved(false);
    setReqs((prev) =>
      prev.map((r, i) => {
        if (i !== index) return r;
        const params = { ...(r.params ?? {}) };
        if (value === '' || value === undefined) delete params[key];
        else params[key] = value;
        return { ...r, params };
      }),
    );
  }

  function addReq() {
    setSaved(false);
    setReqs((prev) => [
      ...prev,
      { target_id: '', kind: MONITOR_KINDS[0]?.kind ?? 'host.basic', params: {} },
    ]);
  }

  function removeReq(index: number) {
    setSaved(false);
    setReqs((prev) => prev.filter((_, i) => i !== index));
  }

  async function handleSave() {
    setError(null);
    try {
      await update.mutateAsync({ ...panel, requirements: reqs });
      setSaved(true);
    } catch (err) {
      setError(errorMessage(err, '保存失败'));
    }
  }

  return (
    <div className="rounded-lg border border-ops-border-subtle bg-ops-surface p-4">
      <div className="flex items-center justify-between">
        <div className="text-sm font-medium text-ops-primary">数据需求</div>
        <Button size="sm" variant="secondary" onClick={addReq} disabled={busy}>
          添加需求
        </Button>
      </div>

      <div className="mt-3 space-y-3">
        {reqs.length === 0 ? (
          <div className="rounded-md border border-dashed border-ops-border-subtle px-3 py-4 text-xs text-ops-tertiary">
            还没有数据需求。监控项需要声明采集什么数据，采集时才会拿到判断依据。
          </div>
        ) : (
          reqs.map((req, index) => {
            const def = kindDef(req.kind);
            const targetOptions = def?.configKind
              ? configs.filter((c) => c.kind === def.configKind)
              : [];
            return (
              <div
                key={index}
                className="space-y-3 rounded-md border border-ops-border-subtle bg-ops-elevated p-3"
              >
                <div className="grid gap-3 md:grid-cols-2">
                  <div className="space-y-1">
                    <Label>数据类型</Label>
                    <Select
                      value={req.kind}
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
                  {def?.configKind ? (
                    <div className="space-y-1">
                      <Label>目标配置（{def.configKind}）</Label>
                      <Select
                        value={req.target_id}
                        onChange={(e) => patchReq(index, { target_id: e.target.value })}
                        disabled={busy}
                      >
                        <option value="">请选择</option>
                        {targetOptions.map((c) => (
                          <option key={c.id} value={c.id}>
                            {c.name}
                          </option>
                        ))}
                      </Select>
                    </div>
                  ) : null}
                </div>

                {def && def.params.length > 0 ? (
                  <div className="grid gap-3 md:grid-cols-2">
                    {def.params.map((field) => (
                      <ParamInput
                        key={field.key}
                        field={field}
                        value={req.params?.[field.key]}
                        disabled={busy}
                        onChange={(v) => setParam(index, field.key, v)}
                      />
                    ))}
                  </div>
                ) : null}

                <div className="flex justify-end">
                  <Button size="sm" variant="ghost" onClick={() => removeReq(index)} disabled={busy}>
                    移除
                  </Button>
                </div>
              </div>
            );
          })
        )}
      </div>

      {error ? (
        <div className="mt-3 rounded border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700">
          {error}
        </div>
      ) : null}

      <div className="mt-4 flex items-center gap-3">
        <Button size="sm" onClick={handleSave} disabled={busy}>
          {busy ? '保存中...' : '保存数据需求'}
        </Button>
        {saved ? <span className="text-xs text-ops-success">已保存</span> : null}
      </div>
    </div>
  );
}

// ParamInput 按字段类型渲染单个参数输入。
function ParamInput({
  field,
  value,
  disabled,
  onChange,
}: {
  field: { key: string; label: string; type: 'text' | 'number' | 'toggle'; placeholder?: string };
  value: unknown;
  disabled: boolean;
  onChange: (value: unknown) => void;
}) {
  if (field.type === 'toggle') {
    return (
      <label className="flex items-center gap-2 text-sm text-ops-secondary">
        <input
          type="checkbox"
          checked={Boolean(value)}
          disabled={disabled}
          onChange={(e) => onChange(e.target.checked)}
        />
        {field.label}
      </label>
    );
  }
  return (
    <div className="space-y-1">
      <Label>{field.label}</Label>
      <Input
        type={field.type === 'number' ? 'number' : 'text'}
        value={value === undefined || value === null ? '' : String(value)}
        placeholder={field.placeholder}
        disabled={disabled}
        onChange={(e) => {
          const raw = e.target.value;
          if (field.type === 'number') onChange(raw === '' ? '' : Number(raw));
          else onChange(raw);
        }}
      />
    </div>
  );
}
