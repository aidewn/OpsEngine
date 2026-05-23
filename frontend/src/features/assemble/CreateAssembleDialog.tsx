// 创建集合弹窗

import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Dialog } from '@/components/ui/Dialog';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Label } from '@/components/ui/Label';
import { Textarea } from '@/components/ui/Textarea';
import { useCreateAssemble, useUpdateAssemble } from '@/api/assembles';
import { createDefaultAssembleNodes } from '@/features/workflow/systemNodes';

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CreateAssembleDialog({ open, onOpenChange }: Props) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [error, setError] = useState<string | null>(null);

  const create = useCreateAssemble();
  const update = useUpdateAssemble();
  const navigate = useNavigate();

  const busy = create.isPending || update.isPending;

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
      // 写入默认 Start + End 节点
      await update.mutateAsync({
        id,
        name: trimmed,
        description: description.trim(),
        params: [],
        returns: [],
        variables: [],
        nodes: createDefaultAssembleNodes(),
        edges: [],
      });
      reset();
      onOpenChange(false);
      navigate(`/assembles/${id}`);
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
      title="新建集合"
      description="创建一个可复用的节点集合，并自动放置 Start / End 节点"
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
          <Button type="submit" form="create-assemble-form" disabled={busy}>
            {busy ? '创建中...' : '创建'}
          </Button>
        </>
      }
    >
      <form
        id="create-assemble-form"
        onSubmit={handleSubmit}
        className="space-y-4"
      >
        <div className="space-y-1">
          <Label htmlFor="assemble-name">名称</Label>
          <Input
            id="assemble-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="例：SSH 部署模块"
            autoFocus
            disabled={busy}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="assemble-desc">描述（可选）</Label>
          <Textarea
            id="assemble-desc"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={3}
            placeholder="说明这个集合的用途"
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
