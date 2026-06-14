// 诊断报告查看弹窗：渲染报告 Markdown 正文。
import { Dialog } from '@/components/ui/Dialog';
import { MarkdownView } from '@/components/ui/MarkdownView';
import type { MonitorReport } from '@/types/monitor';

export function MonitorReportDialog({
  report,
  open,
  onOpenChange,
}: {
  report: MonitorReport | undefined;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={report?.title ?? '诊断报告'}
      description={report ? new Date(report.created_at).toLocaleString() : undefined}
      size="lg"
    >
      {!report ? (
        <div className="text-sm text-ops-tertiary">报告不存在</div>
      ) : report.content ? (
        <MarkdownView body={report.content} />
      ) : (
        <div className="text-sm text-ops-secondary">{report.summary}</div>
      )}
    </Dialog>
  );
}
