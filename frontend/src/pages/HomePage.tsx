// 首页主区：阶段 2 改为 Chat / 工作流 / 报告三类入口。
import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { ErrorBoundary } from '@/components/ErrorBoundary';
import { Button } from '@/components/ui/Button';
import { AIAssistantPanel } from '@/features/ai/AIAssistantDialog';
import { CreateAssembleDialog } from '@/features/assemble/CreateAssembleDialog';
import { OpsDocList } from '@/features/opsDocs/OpsDocList';
import { CreateWorkflowDialog } from '@/features/workflow/CreateWorkflowDialog';

type HomeTab = 'chat' | 'workflow' | 'reports';

// HomePage 根据 query tab 展示三类主视图，保留刷新后的上下文。
export function HomePage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const tab = getHomeTab(searchParams.get('tab'));
  const selectedSessionID = searchParams.get('session');
  const focusAI = searchParams.get('focus') === 'ai';

  const content = useMemo(() => {
    if (tab === 'workflow') return <WorkflowOverview />;
    if (tab === 'reports') return <OpsDocList />;
    return (
      <AIAssistantPanel
        embedded
        selectedSessionID={selectedSessionID}
        onSelectedSessionChange={(id) => {
          const next = new URLSearchParams(searchParams);
          next.set('tab', 'chat');
          next.delete('focus');
          if (id) {
            next.set('session', id);
          } else {
            next.delete('session');
          }
          setSearchParams(next);
        }}
        focusInputOnOpen={focusAI}
        showSessionSidebar={false}
        className="h-full min-h-0"
      />
    );
  }, [focusAI, searchParams, selectedSessionID, setSearchParams, tab]);

  return (
    <div className={tab === 'chat' ? 'h-full min-h-0' : 'h-full overflow-auto px-6 py-6'}>
      <ErrorBoundary>{content}</ErrorBoundary>
    </div>
  );
}

function WorkflowOverview() {
  const [workflowOpen, setWorkflowOpen] = useState(false);
  const [assembleOpen, setAssembleOpen] = useState(false);

  return (
    <>
      <div className="flex h-full min-h-[520px] items-center justify-center">
        <div className="w-full max-w-2xl rounded-lg border border-ops-border-subtle bg-ops-surface p-6">
          <div className="text-xl font-semibold text-ops-primary">工作流</div>
          <p className="mt-2 text-sm text-ops-secondary">
            从左侧选择一个工作流或集合查看详情，也可以创建新的运维编排。
          </p>
          <div className="mt-5 flex flex-wrap gap-2">
            <Button onClick={() => setWorkflowOpen(true)}>新建工作流</Button>
            <Button variant="secondary" onClick={() => setAssembleOpen(true)}>
              新建集合
            </Button>
          </div>
        </div>
      </div>
      <CreateWorkflowDialog open={workflowOpen} onOpenChange={setWorkflowOpen} />
      <CreateAssembleDialog open={assembleOpen} onOpenChange={setAssembleOpen} />
    </>
  );
}

function getHomeTab(tab: string | null): HomeTab {
  if (tab === 'workflow' || tab === 'reports') return tab;
  return 'chat';
}
