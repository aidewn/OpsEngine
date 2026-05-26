// 创建环境弹窗
// 仅创建空环境记录，跳转到详情页继续添加 configs

import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Dialog } from '@/components/ui/Dialog';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Label } from '@/components/ui/Label';
import { Textarea } from '@/components/ui/Textarea';
import { useCreateEnvironment } from '@/api/environments';

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CreateEnvironmentDialog({ open, onOpenChange }: Props) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [error, setError] = useState<string | null>(null);

  const create = useCreateEnvironment();
  const navigate = useNavigate();

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
      setError('名称不能为空');
      return;
    }

    try {
      const { id } = await create.mutateAsync({
        name: trimmed,
        description: description.trim(),
      });
      reset();
      onOpenChange(false);
      navigate(`/environments/${id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建失败');
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
      title="新建环境"
      description="例：nginx 生产、yum 测试环境"
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
          <Button type="submit" form="create-environment-form" disabled={busy}>
            {busy ? '创建中...' : '创建'}
          </Button>
        </>
      }
    >
      <form
        id="create-environment-form"
        onSubmit={handleSubmit}
        className="space-y-4"
      >
        <div className="space-y-1">
          <Label htmlFor="env-name">名称</Label>
          <Input
            id="env-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="例：nginx 生产"
            autoFocus
            disabled={busy}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="env-desc">描述（可选）</Label>
          <Textarea
            id="env-desc"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={3}
            placeholder="说明这个环境的用途"
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
