// ExecutionFinishedNotifier 组件：监听执行完成事件并提供生成报告的就地反馈。
import { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { EventsOn } from '@wails/runtime/runtime';
import { useGenerateInspectionReport } from '@/api/opsDocs';
import { toast } from '@/lib/toast';
import { hasWailsRuntime } from '@/lib/wailsRuntime';

interface FinishedPayload {
  executionID: string;
  status: string;
  error?: string;
}

export function ExecutionFinishedNotifier() {
  const navigate = useNavigate();
  const generateReport = useGenerateInspectionReport();

  useEffect(() => {
    if (!hasWailsRuntime()) return undefined;

    const off = EventsOn('execution:finished', (payload: FinishedPayload) => {
      if (payload.status === 'Success') {
        toast.success('执行成功', {
          label: '生成巡检报告',
          onClick: () => {
            generateReport.mutate(payload.executionID, {
              onSuccess: (doc) => {
                navigate(`/?tab=reports&doc=${doc.id}`);
                toast.success('报告已生成');
              },
              onError: (err) => {
                toast.error(err.message || '生成报告失败');
              },
            });
          },
        });
        return;
      }

      if (payload.status === 'Failed') {
        toast.error('执行失败', payload.error);
      }
    });

    return off;
  }, [generateReport, navigate]);

  return null;
}
