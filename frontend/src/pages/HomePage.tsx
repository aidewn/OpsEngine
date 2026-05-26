// 首页：顶部 tab 切换工作流/集合列表

import { useState } from 'react';
import { cn } from '@/lib/cn';
import { ErrorBoundary } from '@/components/ErrorBoundary';
import { WorkflowList } from '@/features/workflow/WorkflowList';
import { AssembleList } from '@/features/assemble/AssembleList';
import { ExecutionList } from '@/features/execution/ExecutionList';
import { EnvironmentList } from '@/features/environment/EnvironmentList';

type Tab = 'workflow' | 'assemble' | 'execution' | 'environment';

export function HomePage() {
  const [tab, setTab] = useState<Tab>('workflow');

  return (
    <div className="mx-auto flex h-full max-w-5xl flex-col px-6 py-8">
      {/* Tab 切换 */}
      <div className="mb-6 flex gap-1 rounded-lg bg-slate-100 p-1 self-start">
        <TabButton
          active={tab === 'workflow'}
          onClick={() => setTab('workflow')}
        >
          工作流
        </TabButton>
        <TabButton
          active={tab === 'assemble'}
          onClick={() => setTab('assemble')}
        >
          集合
        </TabButton>
        <TabButton
          active={tab === 'execution'}
          onClick={() => setTab('execution')}
        >
          执行
        </TabButton>
        <TabButton
          active={tab === 'environment'}
          onClick={() => setTab('environment')}
        >
          配置环境
        </TabButton>
      </div>

      {/* 列表内容；包 ErrorBoundary 避免单个 tab 渲染异常打挂整页 */}
      <ErrorBoundary>
        {tab === 'workflow' && <WorkflowList />}
        {tab === 'assemble' && <AssembleList />}
        {tab === 'execution' && <ExecutionList />}
        {tab === 'environment' && <EnvironmentList />}
      </ErrorBoundary>
    </div>
  );
}

// Tab 按钮
function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'rounded-md px-4 py-1.5 text-sm font-medium transition-colors',
        active
          ? 'bg-white text-slate-900 shadow-sm'
          : 'text-slate-500 hover:text-slate-700',
      )}
    >
      {children}
    </button>
  );
}
