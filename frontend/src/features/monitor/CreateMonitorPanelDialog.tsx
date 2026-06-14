// 新建监控项弹窗：在当前环境的某个分组下创建一个监控项。
// 监控项对应一个用户关心的问题（如「CPU 使用率是否大于 80%」），而非一条原始指标。
import { useEffect, useState } from 'react';
import { Button } from '@/components/ui/Button';
import { Dialog } from '@/components/ui/Dialog';
import { Input } from '@/components/ui/Input';
import { Label } from '@/components/ui/Label';
import { Select } from '@/components/ui/Select';
import { Textarea } from '@/components/ui/Textarea';
import { useCreateMonitorPanel } from '@/api/monitor';
import { errorMessage } from '@/lib/error';
import type { MonitorGroup } from '@/types/monitor';

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  environmentID: string;
  groups: MonitorGroup[];
  // defaultGroupID 在从分组详情页打开时预选该分组。
  defaultGroupID?: string;
}

export function CreateMonitorPanelDialog({
  open,
  onOpenChange,
  environmentID,
  groups,
  defaultGroupID,
}: Props) {
  const [groupID, setGroupID] = useState(defaultGroupID ?? groups[0]?.id ?? '');
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [error, setError] = useState<string | null>(null);

  const create = useCreateMonitorPanel();
  const busy = create.isPending;

  // 弹窗打开时同步预选分组（defaultGroupID 或首个分组）。
  useEffect(() => {
    if (open) {
      setGroupID(defaultGroupID ?? groups[0]?.id ?? '');
    }
  }, [open, defaultGroupID, groups]);

  function reset() {
    setName('');
    setDescription('');
    setError(null);
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const trimmed = name.trim();
    if (!groupID) {
      setError('请先选择分组');
      return;
    }
    if (!trimmed) {
      setError('监控项名称不能为空');
      return;
    }
    try {
      await create.mutateAsync({
        environmentID,
        groupID,
        name: trimmed,
        description: description.trim(),
      });
      reset();
      onOpenChange(false);
    } catch (err) {
      setError(errorMessage(err, '创建失败'));
    }
  }

  const noGroups = groups.length === 0;

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!busy) {
          if (!v) reset();
          onOpenChange(v);
        }
      }}
      title="新建监控项"
      description="对应一个具体的监控问题，如「磁盘水位是否大于 85%」"
      footer={
        <>
          <Button type="button" variant="secondary" onClick={() => onOpenChange(false)} disabled={busy}>
            取消
          </Button>
          <Button type="submit" form="create-monitor-panel-form" disabled={busy || noGroups}>
            {busy ? '创建中...' : '创建'}
          </Button>
        </>
      }
    >
      {noGroups ? (
        <div className="rounded border border-ops-border-subtle bg-ops-elevated px-3 py-4 text-sm text-ops-secondary">
          当前环境还没有分组，请先创建一个分组再添加监控项。
        </div>
      ) : (
        <form id="create-monitor-panel-form" onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1">
            <Label htmlFor="monitor-panel-group">分组</Label>
            <Select
              id="monitor-panel-group"
              value={groupID}
              onChange={(e) => setGroupID(e.target.value)}
              disabled={busy}
            >
              {groups.map((group) => (
                <option key={group.id} value={group.id}>
                  {group.name}
                </option>
              ))}
            </Select>
          </div>
          <div className="space-y-1">
            <Label htmlFor="monitor-panel-name">名称</Label>
            <Input
              id="monitor-panel-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例：CPU 使用率是否大于 80%"
              autoFocus
              disabled={busy}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="monitor-panel-desc">描述（可选）</Label>
            <Textarea
              id="monitor-panel-desc"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
              placeholder="说明这个监控项判断什么、依赖哪些数据"
              disabled={busy}
            />
          </div>
          {error && (
            <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700">
              {error}
            </div>
          )}
        </form>
      )}
    </Dialog>
  );
}
