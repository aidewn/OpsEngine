// 巡检报告弹窗：展示已有 OpsDoc，支持生成/重新生成/复制/删除。
// Markdown 渲染由共享组件 @/components/ui/MarkdownView 提供，文档库页面也会复用。

import { useEffect, useMemo, useState } from 'react';
import { Dialog } from '@/components/ui/Dialog';
import { Button } from '@/components/ui/Button';
import { MarkdownView } from '@/components/ui/MarkdownView';
import {
  useDeleteOpsDoc,
  useGenerateInspectionReport,
  useOpsDoc,
  useOpsDocs,
} from '@/api/opsDocs';
import type { OpsDoc } from '@/types/opsDoc';

export function InspectionReportDialog({
  open,
  executionID,
  onOpenChange,
}: {
  open: boolean;
  executionID: string | undefined;
  onOpenChange: (open: boolean) => void;
}) {
  const generate = useGenerateInspectionReport();
  const deleteDoc = useDeleteOpsDoc();
  const { data: docs } = useOpsDocs();
  const [doc, setDoc] = useState<OpsDoc | null>(null);
  const [selectedDocID, setSelectedDocID] = useState<string | undefined>();
  const { data: loadedDoc } = useOpsDoc(selectedDocID);
  const [error, setError] = useState<string | null>(null);
  const [copyText, setCopyText] = useState('复制');

  const relatedDocs = useMemo(
    () =>
      (docs ?? []).filter(
        (item) =>
          item.kind === 'inspection' &&
          !!executionID &&
          item.source.execution_id === executionID,
      ),
    [docs, executionID],
  );

  // 打开时优先选中已有报告；没有报告时等待用户点击生成，避免每次打开都重复落盘。
  useEffect(() => {
    if (!open || !executionID) return;
    setError(null);
    setSelectedDocID(relatedDocs[0]?.id);
    if (relatedDocs.length === 0) {
      setDoc(null);
    }
  }, [open, executionID, relatedDocs]);

  useEffect(() => {
    if (loadedDoc) setDoc(loadedDoc);
  }, [loadedDoc]);

  useEffect(() => {
    if (!open) {
      setDoc(null);
      setSelectedDocID(undefined);
      setError(null);
      setCopyText('复制');
    }
  }, [open]);

  const busy = generate.isPending || deleteDoc.isPending;

  async function handleGenerate() {
    if (!executionID || busy) return;
    setError(null);
    try {
      const next = await generate.mutateAsync(executionID);
      setDoc(next);
      setSelectedDocID(next.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function handleCopy() {
    if (!doc?.body) return;
    try {
      await navigator.clipboard.writeText(doc.body);
      setCopyText('已复制');
      window.setTimeout(() => setCopyText('复制'), 1200);
    } catch {
      setCopyText('复制失败');
      window.setTimeout(() => setCopyText('复制'), 1200);
    }
  }

  async function handleDelete() {
    if (!doc || busy) return;
    if (!confirm('确认删除这份巡检报告？')) return;
    setError(null);
    try {
      await deleteDoc.mutateAsync(doc.id);
      setDoc(null);
      setSelectedDocID(undefined);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!busy) onOpenChange(next);
      }}
      title="巡检报告"
      description="基于执行结果自动生成。若已配置 LLM API Key，会附加风险与建议段。"
      contentClassName="max-w-3xl"
    >
      <div className="flex max-h-[70vh] flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            {relatedDocs.length > 0 && (
              <select
                value={selectedDocID ?? ''}
                onChange={(event) => setSelectedDocID(event.target.value || undefined)}
                className="h-8 max-w-[320px] rounded-md border border-slate-300 bg-white px-2 text-xs text-slate-700 outline-none focus:border-slate-500"
                disabled={busy}
              >
                {relatedDocs.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.title} · {new Date(item.updated_at).toLocaleString()}
                  </option>
                ))}
              </select>
            )}
            {doc && (
              <span className="truncate text-xs text-slate-500">
                文档 ID: <span className="font-mono">{doc.id.slice(0, 8)}</span>
              </span>
            )}
          </div>
          <div className="flex items-center gap-2">
            {doc && (
              <>
                <Button variant="ghost" size="sm" onClick={handleCopy} disabled={busy}>
                  {copyText}
                </Button>
                <Button variant="ghost" size="sm" onClick={handleDelete} disabled={busy}>
                  删除
                </Button>
              </>
            )}
            <Button size="sm" onClick={handleGenerate} disabled={busy || !executionID}>
              {doc ? '重新生成' : '生成报告'}
            </Button>
          </div>
        </div>

        {busy && (
          <div className="rounded-md bg-slate-50 px-3 py-4 text-sm text-slate-500">
            正在生成报告（可能需要 5–30 秒，LLM 风险分析耗时取决于模型）…
          </div>
        )}
        {error && (
          <div className="rounded-md bg-rose-50 px-3 py-2 text-sm text-rose-700">
            生成失败：{error}
          </div>
        )}
        {!busy && !doc && !error && (
          <div className="rounded-md border border-dashed border-slate-300 bg-slate-50 px-3 py-8 text-center text-sm text-slate-500">
            还没有为本次执行生成巡检报告。
          </div>
        )}
        {doc && (
          <>
            <div className="flex-1 overflow-auto rounded-md border border-slate-200 bg-white px-4 py-3">
              <MarkdownView body={doc.body} />
            </div>
          </>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => onOpenChange(false)} disabled={busy}>
            关闭
          </Button>
        </div>
      </div>
    </Dialog>
  );
}

