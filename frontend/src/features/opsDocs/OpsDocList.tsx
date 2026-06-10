// OpsDocList 组件：报告 tab 主区只渲染当前选中文档详情。
// 文档筛选和列表选择统一交给全局 Sidebar，避免主区重复导航。
import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Button } from '@/components/ui/Button';
import { MarkdownView } from '@/components/ui/MarkdownView';
import { useDeleteOpsDoc, useOpsDoc, useOpsDocs } from '@/api/opsDocs';
import type { OpsDocKind } from '@/types/opsDoc';
import { toast } from '@/lib/toast';

export function OpsDocList() {
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedID = searchParams.get('doc') ?? undefined;
  const { data: docs, isLoading } = useOpsDocs();
  const { data: detail } = useOpsDoc(selectedID);
  const deleteDoc = useDeleteOpsDoc();
  const [copyText, setCopyText] = useState('复制全文');

  useEffect(() => {
    setCopyText('复制全文');
  }, [selectedID]);

  async function handleCopy() {
    if (!detail?.body) return;
    try {
      await navigator.clipboard.writeText(detail.body);
      setCopyText('已复制');
      window.setTimeout(() => setCopyText('复制全文'), 1200);
    } catch {
      setCopyText('复制失败');
      window.setTimeout(() => setCopyText('复制全文'), 1200);
    }
  }

  async function handleDelete() {
    if (!detail || deleteDoc.isPending) return;
    if (!confirm(`确认删除文档「${detail.title}」？此操作不可撤销。`)) return;
    try {
      await deleteDoc.mutateAsync(detail.id);
      setSearchParams({ tab: 'reports' });
      toast.success('文档已删除');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    }
  }

  if (isLoading) {
    return <div className="py-12 text-center text-sm text-ops-secondary">加载中…</div>;
  }

  if (!docs || docs.length === 0) {
    return (
      <EmptyReportState
        title="还没有文档"
        description="执行一次巡检工作流后，在执行详情中生成第一份报告。"
      />
    );
  }

  if (!selectedID) {
    return (
      <EmptyReportState
        title="选择一份报告"
        description="从左侧报告列表选择文档后，这里会显示完整 Markdown 内容。"
      />
    );
  }

  if (!detail) {
    return <div className="py-12 text-center text-sm text-ops-secondary">正在加载文档详情…</div>;
  }

  return (
    <div className="flex h-full min-h-0 flex-col rounded-md border border-ops-border-subtle bg-ops-surface">
      <header className="flex items-center justify-between border-b border-ops-border-subtle px-4 py-3">
        <div className="min-w-0">
          <div className="truncate text-base font-medium text-ops-primary">{detail.title}</div>
          <div className="mt-0.5 text-2xs text-ops-secondary">
            {kindLabel(detail.kind)} · 更新于 {new Date(detail.updated_at).toLocaleString()} ·{' '}
            <span className="font-mono">{detail.id.slice(0, 8)}</span>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button variant="ghost" size="sm" onClick={handleCopy}>
            {copyText}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={handleDelete}
            disabled={deleteDoc.isPending}
          >
            删除
          </Button>
        </div>
      </header>
      <div className="min-h-0 flex-1 overflow-auto px-4 py-3">
        <MarkdownView body={detail.body} />
      </div>
    </div>
  );
}

function EmptyReportState({ title, description }: { title: string; description: string }) {
  return (
    <div className="flex h-full min-h-[520px] items-center justify-center">
      <div className="w-full max-w-2xl rounded-lg border border-dashed border-ops-border-subtle bg-ops-surface px-6 py-16 text-center">
        <div className="text-base font-medium text-ops-primary">{title}</div>
        <div className="mt-2 text-sm text-ops-secondary">{description}</div>
      </div>
    </div>
  );
}

// kindLabel 把 OpsDocKind 映射成中文短标签。
function kindLabel(kind: OpsDocKind): string {
  switch (kind) {
    case 'inspection':
      return '巡检';
    case 'troubleshooting':
      return '排障';
    case 'architecture':
      return '架构';
    default:
      return kind;
  }
}
