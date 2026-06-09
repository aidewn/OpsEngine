// 文档库视图：左侧文档列表 + 右侧 Markdown 详情。
// 支持按 Kind 过滤、复制全文、删除。空状态提示用户从执行详情生成第一份报告。

import { useEffect, useMemo, useState } from 'react';
import { Button } from '@/components/ui/Button';
import { MarkdownView } from '@/components/ui/MarkdownView';
import {
  useDeleteOpsDoc,
  useOpsDoc,
  useOpsDocs,
} from '@/api/opsDocs';
import type { OpsDocKind, OpsDocSummary } from '@/types/opsDoc';
import { cn } from '@/lib/cn';

// kindLabels 控制 tab 顺序与中文标签。
const KIND_TABS: Array<{ value: 'all' | OpsDocKind; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'inspection', label: '巡检' },
  { value: 'troubleshooting', label: '排障' },
  { value: 'architecture', label: '架构' },
];

export function OpsDocList() {
  const { data: docs, isLoading } = useOpsDocs();
  const deleteDoc = useDeleteOpsDoc();
  const [filter, setFilter] = useState<'all' | OpsDocKind>('all');
  const [selectedID, setSelectedID] = useState<string | undefined>();
  const { data: detail } = useOpsDoc(selectedID);
  const [copyText, setCopyText] = useState('复制');

  const filtered = useMemo(() => {
    const all = docs ?? [];
    if (filter === 'all') return all;
    return all.filter((d) => d.kind === filter);
  }, [docs, filter]);

  // 首次加载或过滤切换后，自动选中第一条；当前选中项被过滤掉时也归零。
  useEffect(() => {
    if (filtered.length === 0) {
      setSelectedID(undefined);
      return;
    }
    if (!selectedID || !filtered.some((d) => d.id === selectedID)) {
      setSelectedID(filtered[0]!.id);
    }
  }, [filtered, selectedID]);

  async function handleCopy() {
    if (!detail?.body) return;
    try {
      await navigator.clipboard.writeText(detail.body);
      setCopyText('已复制');
      window.setTimeout(() => setCopyText('复制'), 1200);
    } catch {
      setCopyText('复制失败');
      window.setTimeout(() => setCopyText('复制'), 1200);
    }
  }

  async function handleDelete() {
    if (!detail || deleteDoc.isPending) return;
    if (!confirm(`确认删除文档「${detail.title}」？此操作不可撤销。`)) return;
    try {
      await deleteDoc.mutateAsync(detail.id);
      setSelectedID(undefined);
    } catch (err) {
      alert(err instanceof Error ? err.message : String(err));
    }
  }

  if (isLoading) {
    return <div className="py-12 text-center text-sm text-slate-500">加载中…</div>;
  }

  const all = docs ?? [];
  if (all.length === 0) {
    return (
      <div className="rounded-md border border-dashed border-slate-300 bg-slate-50 px-6 py-16 text-center text-sm text-slate-500">
        <div className="mb-1 font-medium text-slate-700">还没有文档</div>
        <div>到执行详情页执行一次巡检工作流后，点击"📄 生成报告"即可创建第一份文档。</div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      {/* Kind 过滤 */}
      <div className="flex flex-wrap items-center gap-2">
        {KIND_TABS.map((tab) => (
          <button
            key={tab.value}
            type="button"
            onClick={() => setFilter(tab.value)}
            className={cn(
              'rounded-md px-3 py-1 text-xs font-medium transition-colors',
              filter === tab.value
                ? 'bg-slate-900 text-white'
                : 'bg-slate-100 text-slate-600 hover:bg-slate-200',
            )}
          >
            {tab.label}
            <span className="ml-1 text-[10px] opacity-70">
              ({tab.value === 'all'
                ? all.length
                : all.filter((d) => d.kind === tab.value).length})
            </span>
          </button>
        ))}
      </div>

      <div className="flex h-[640px] gap-4">
        {/* 左侧列表 */}
        <div className="w-72 shrink-0 overflow-auto rounded-md border border-slate-200 bg-white">
          {filtered.length === 0 ? (
            <div className="p-4 text-center text-xs text-slate-400">该分类下暂无文档</div>
          ) : (
            <ul>
              {filtered.map((doc) => (
                <DocItem
                  key={doc.id}
                  doc={doc}
                  selected={doc.id === selectedID}
                  onSelect={() => setSelectedID(doc.id)}
                />
              ))}
            </ul>
          )}
        </div>

        {/* 右侧详情 */}
        <div className="flex flex-1 flex-col overflow-hidden rounded-md border border-slate-200 bg-white">
          {!detail ? (
            <div className="flex-1 flex items-center justify-center text-sm text-slate-400">
              请选择一份文档查看
            </div>
          ) : (
            <>
              <div className="flex items-center justify-between border-b border-slate-200 px-4 py-2">
                <div className="min-w-0">
                  <div className="truncate text-sm font-medium text-slate-900">{detail.title}</div>
                  <div className="mt-0.5 text-[11px] text-slate-500">
                    {kindLabel(detail.kind)} · 更新于{' '}
                    {new Date(detail.updated_at).toLocaleString()} ·{' '}
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
              </div>
              <div className="flex-1 overflow-auto px-4 py-3">
                <MarkdownView body={detail.body} />
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

// DocItem 渲染左侧列表的单项。
function DocItem({
  doc,
  selected,
  onSelect,
}: {
  doc: OpsDocSummary;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <li>
      <button
        type="button"
        onClick={onSelect}
        className={cn(
          'block w-full border-b border-slate-100 px-3 py-2 text-left text-xs transition-colors last:border-b-0',
          selected ? 'bg-slate-100' : 'hover:bg-slate-50',
        )}
      >
        <div className="truncate text-sm font-medium text-slate-900">{doc.title}</div>
        <div className="mt-0.5 flex items-center gap-1.5 text-[11px] text-slate-500">
          <span className="rounded-sm bg-slate-200 px-1 py-0.5 text-[10px] text-slate-700">
            {kindLabel(doc.kind)}
          </span>
          <span>{new Date(doc.updated_at).toLocaleString()}</span>
        </div>
      </button>
    </li>
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
