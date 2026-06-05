// 巡检报告弹窗：触发生成 + 展示 Markdown 正文。
// 当前用 <pre> 渲染原文 Markdown，后续若需可视化可换 react-markdown。

import { useEffect, useState } from 'react';
import { Dialog } from '@/components/ui/Dialog';
import { Button } from '@/components/ui/Button';
import { useGenerateInspectionReport } from '@/api/opsDocs';
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
  const [doc, setDoc] = useState<OpsDoc | null>(null);
  const [error, setError] = useState<string | null>(null);

  // 打开时自动触发一次生成；关闭时清掉本地状态。
  useEffect(() => {
    if (!open || !executionID) return;
    setDoc(null);
    setError(null);
    generate
      .mutateAsync(executionID)
      .then(setDoc)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err));
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, executionID]);

  useEffect(() => {
    if (!open) {
      setDoc(null);
      setError(null);
    }
  }, [open]);

  const busy = generate.isPending;
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
        {doc && (
          <>
            <div className="text-xs text-slate-500">
              已保存到本地文档库 · ID:&nbsp;
              <span className="font-mono">{doc.id.slice(0, 8)}</span>
            </div>
            <pre className="flex-1 overflow-auto whitespace-pre-wrap rounded-md border border-slate-200 bg-slate-50 px-3 py-2 font-mono text-xs leading-relaxed text-slate-800">
              {doc.body}
            </pre>
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
