// 工作流编辑页右上角的「运行中副本」小卡片
// 点击展开下拉，列出该工作流的所有 execution（运行中优先）
// 点击列表项打开对应 execution tab

import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useExecutionsByWorkflow } from '@/api/executions';
import { WorkflowStatusIcon } from './ExecutionStatus';
import { useTabs } from '@/features/tabs/TabsContext';
import { cn } from '@/lib/cn';

interface Props {
  workflowID: string;
}

export function RunningBadge({ workflowID }: Props) {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();
  const { openTab } = useTabs();
  const { data } = useExecutionsByWorkflow(workflowID);
  const execs = Array.isArray(data) ? data : [];

  const runningCount = execs.filter((e) => e.status === 'Running').length;
  if (execs.length === 0) return null;

  function handleSelect(execID: string, workflowName: string) {
    openTab({
      kind: 'execution',
      id: execID,
      name: `${workflowName} #${execID.slice(0, 4)}`,
    });
    navigate(`/executions/${execID}`);
    setOpen(false);
  }

  return (
    <div className="relative">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex items-center gap-1.5 rounded border border-slate-200 bg-white px-2 py-1 text-xs hover:bg-slate-50"
      >
        {runningCount > 0 ? (
          <>
            <span className="inline-block size-1.5 animate-pulse rounded-full bg-blue-500" />
            <span className="text-slate-700">运行中 {runningCount}</span>
          </>
        ) : (
          <span className="text-slate-500">历史 {execs.length}</span>
        )}
        <span className="text-slate-400">▾</span>
      </button>

      {open && (
        <>
          {/* 点击遮罩关闭 */}
          <div
            className="fixed inset-0 z-10"
            onClick={() => setOpen(false)}
          />
          <div className="absolute right-0 top-full z-20 mt-1 w-72 rounded-md border border-slate-200 bg-white shadow-lg">
            <div className="max-h-64 overflow-y-auto py-1">
              {execs.map((e) => (
                <button
                  key={e.id}
                  type="button"
                  onClick={() => handleSelect(e.id, e.workflow_name)}
                  className={cn(
                    'flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs hover:bg-slate-50',
                  )}
                >
                  <WorkflowStatusIcon status={e.status} size={12} />
                  <span className="font-mono text-slate-700">
                    #{e.id.slice(0, 6)}
                  </span>
                  <span className="flex-1 truncate text-slate-500">
                    {formatTime(e.started_at)}
                  </span>
                  <span className="text-[10px] text-slate-400">
                    {e.status}
                  </span>
                </button>
              ))}
            </div>
          </div>
        </>
      )}
    </div>
  );
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleTimeString('zh-CN', { hour12: false });
  } catch {
    return iso;
  }
}
