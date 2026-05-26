// 环境配置项新增 / 编辑对话框
// editItem 为 null 表示新增；非 null 表示编辑（kind 不可改）

import { useEffect, useState } from 'react';
import { Dialog } from '@/components/ui/Dialog';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Label } from '@/components/ui/Label';
import { Textarea } from '@/components/ui/Textarea';
import { cn } from '@/lib/cn';
import { useUpdateEnvironment } from '@/api/environments';
import type {
  EnvConfigItem,
  EnvConfigKind,
  EnvironmentDef,
} from '@/types/environment';
import { EnvConfigForm, defaultFieldsForKind } from './EnvConfigForm';

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  environment: EnvironmentDef;
  // null = 新增；存在 = 编辑既有项
  editItem: EnvConfigItem | null;
}

// 四种 kind 全部可选；MVP 仅 SSH 表单可填，其它 kind 落库但表单显示占位
const KIND_OPTIONS: { value: EnvConfigKind; label: string }[] = [
  { value: 'ssh', label: 'SSH' },
  { value: 'docker', label: 'Docker' },
  { value: 'k8s', label: 'K8s' },
  { value: 'jenkins', label: 'Jenkins' },
];

export function EnvConfigDialog({
  open,
  onOpenChange,
  environment,
  editItem,
}: Props) {
  const update = useUpdateEnvironment();

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [kind, setKind] = useState<EnvConfigKind>('ssh');
  const [fields, setFields] = useState<Record<string, unknown>>({});
  const [error, setError] = useState<string | null>(null);

  // 打开时根据 editItem 初始化 / 重置
  useEffect(() => {
    if (!open) return;
    if (editItem) {
      setName(editItem.name);
      setDescription(editItem.description);
      setKind(editItem.kind);
      setFields({ ...editItem.fields });
    } else {
      setName('');
      setDescription('');
      setKind('ssh');
      setFields(defaultFieldsForKind('ssh'));
    }
    setError(null);
  }, [open, editItem]);

  // 新增模式切换 kind 时重置 fields 为该 kind 的默认值
  // 编辑模式 kind 锁定，不会进入这个分支
  function handleKindChange(next: EnvConfigKind) {
    setKind(next);
    setFields(defaultFieldsForKind(next));
  }

  const busy = update.isPending;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    const trimmed = name.trim();
    if (!trimmed) {
      setError('名称不能为空');
      return;
    }

    // 组装新的 configs 数组：编辑模式替换同 ID 项，新增模式追加
    let nextConfigs: EnvConfigItem[];
    if (editItem) {
      nextConfigs = environment.configs.map((c) =>
        c.id === editItem.id
          ? { ...c, name: trimmed, description: description.trim(), fields }
          : c,
      );
    } else {
      const newItem: EnvConfigItem = {
        id: crypto.randomUUID(),
        name: trimmed,
        kind,
        description: description.trim(),
        fields,
      };
      nextConfigs = [...environment.configs, newItem];
    }

    try {
      await update.mutateAsync({ ...environment, configs: nextConfigs });
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存失败');
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!busy) onOpenChange(v);
      }}
      title={editItem ? '编辑配置' : '添加配置'}
      footer={
        <>
          <Button
            type="button"
            variant="secondary"
            onClick={() => onOpenChange(false)}
            disabled={busy}
          >
            取消
          </Button>
          <Button type="submit" form="env-config-form" disabled={busy}>
            {busy ? '保存中...' : '保存'}
          </Button>
        </>
      }
    >
      <form
        id="env-config-form"
        onSubmit={handleSubmit}
        className="space-y-4"
      >
        <div className="space-y-1">
          <Label htmlFor="env-config-name">名称</Label>
          <Input
            id="env-config-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="例：应用机"
            autoFocus
            disabled={busy}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="env-config-kind">类型</Label>
          <select
            id="env-config-kind"
            value={kind}
            onChange={(e) => handleKindChange(e.target.value as EnvConfigKind)}
            disabled={busy || !!editItem}
            className={cn(
              'w-full rounded border border-slate-300 bg-white px-2 py-1.5 text-xs',
              editItem && 'cursor-not-allowed text-slate-500',
            )}
          >
            {KIND_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
          {editItem && (
            <div className="text-[11px] text-slate-400">
              类型创建后不可更改
            </div>
          )}
        </div>
        <div className="space-y-1">
          <Label htmlFor="env-config-desc">描述（可选）</Label>
          <Textarea
            id="env-config-desc"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={2}
            disabled={busy}
          />
        </div>

        <div className="rounded border border-slate-200 bg-slate-50 p-3">
          <EnvConfigForm
            kind={kind}
            value={fields}
            onChange={setFields}
            environment={environment}
            editingItemID={editItem?.id}
          />
        </div>

        {error && (
          <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700">
            {error}
          </div>
        )}
      </form>
    </Dialog>
  );
}
