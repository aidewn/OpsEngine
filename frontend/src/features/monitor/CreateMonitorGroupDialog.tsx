// 新建监控分组弹窗：在当前环境下创建一个分组，成功后由概览缓存失效自动刷新。
import { useState } from 'react';
import { Button } from '@/components/ui/Button';
import { Dialog } from '@/components/ui/Dialog';
import { Input } from '@/components/ui/Input';
import { Label } from '@/components/ui/Label';
import { Textarea } from '@/components/ui/Textarea';
import { useCreateMonitorGroup } from '@/api/monitor';
import { errorMessage } from '@/lib/error';

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  environmentID: string;
}

export function CreateMonitorGroupDialog({ open, onOpenChange, environmentID }: Props) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [error, setError] = useState<string | null>(null);

  const create = useCreateMonitorGroup();
  const busy = create.isPending;

  function reset() {
    setName('');
    setDescription('');
    setError(null);
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const trimmed = name.trim();
    if (!trimmed) {
      setError('分组名称不能为空');
      return;
    }
    try {
      await create.mutateAsync({ environmentID, name: trimmed, description: description.trim() });
      reset();
      onOpenChange(false);
    } catch (err) {
      setError(errorMessage(err, '创建失败'));
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!busy) {
          if (!v) reset();
          onOpenChange(v);
        }
      }}
      title="新建分组"
      description="把同一环境下的监控项按业务、主机或职责归类"
      footer={
        <>
          <Button type="button" variant="secondary" onClick={() => onOpenChange(false)} disabled={busy}>
            取消
          </Button>
          <Button type="submit" form="create-monitor-group-form" disabled={busy}>
            {busy ? '创建中...' : '创建'}
          </Button>
        </>
      }
    >
      <form id="create-monitor-group-form" onSubmit={handleSubmit} className="space-y-4">
        <div className="space-y-1">
          <Label htmlFor="monitor-group-name">名称</Label>
          <Input
            id="monitor-group-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="例：基础资源"
            autoFocus
            disabled={busy}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="monitor-group-desc">描述（可选）</Label>
          <Textarea
            id="monitor-group-desc"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={3}
            placeholder="说明这个分组涵盖哪些监控项"
            disabled={busy}
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
