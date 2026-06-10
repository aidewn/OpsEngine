// Sidebar 组件：阶段 2 的三 tab 侧栏，负责列表导航和折叠状态。
import { useEffect, useMemo, useState } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { ArrowLeftToLine, ArrowRightToLine, FileText, GitBranch, Layers, MessageSquare } from 'lucide-react';
import { useAISessions, useDeleteAISession } from '@/api/ai';
import { useAssembles, useDeleteAssemble } from '@/api/assembles';
import { useDeleteOpsDoc, useOpsDocs } from '@/api/opsDocs';
import { useDeleteWorkflow, useWorkflows } from '@/api/workflows';
import { toast } from '@/lib/toast';
import { CreateAssembleDialog } from '@/features/assemble/CreateAssembleDialog';
import { CreateWorkflowDialog } from '@/features/workflow/CreateWorkflowDialog';
import type { OpsDocKind } from '@/types/opsDoc';
import { SidebarFilter } from './SidebarFilter';
import { SidebarItem } from './SidebarItem';
import { SidebarList } from './SidebarList';
import { SidebarTabBar, type SidebarTab } from './SidebarTabBar';

type WorkflowFilter = 'workflow' | 'assemble';
type ReportFilter = 'all' | OpsDocKind;

// Sidebar 根据 URL 推导当前主 tab，避免刷新后丢上下文。
export function Sidebar() {
  const location = useLocation();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [collapsed, setCollapsed] = useState(false);
  const [workflowFilter, setWorkflowFilter] = useState<WorkflowFilter>('workflow');
  const [reportFilter, setReportFilter] = useState<ReportFilter>('all');

  const activeTab = getActiveSidebarTab(location.pathname, searchParams.get('tab'));

  useEffect(() => {
    if (location.pathname.startsWith('/assembles/')) {
      setWorkflowFilter('assemble');
    } else if (location.pathname.startsWith('/workflows/')) {
      setWorkflowFilter('workflow');
    }
  }, [location.pathname]);

  function handleTabChange(tab: SidebarTab) {
    navigate(`/?tab=${tab}`);
  }

  return (
    <aside
      className="flex h-full shrink-0 flex-col border-r border-ops-border-subtle bg-ops-sidebar"
      style={{ width: collapsed ? 56 : 240 }}
    >
      <div className="flex h-10 items-center justify-between border-b border-ops-border-subtle px-2">
        {!collapsed ? <span className="text-xs font-medium text-ops-secondary">OpsEngine</span> : null}
        <button
          type="button"
          className="titlebar-no-drag titlebar-icon-button"
          title={collapsed ? '展开侧栏' : '折叠侧栏'}
          aria-label={collapsed ? '展开侧栏' : '折叠侧栏'}
          onClick={() => setCollapsed((value) => !value)}
        >
          {collapsed ? <ArrowRightToLine size={14} /> : <ArrowLeftToLine size={14} />}
        </button>
      </div>
      <SidebarTabBar activeTab={activeTab} collapsed={collapsed} onTabChange={handleTabChange} />
      <div className="min-h-0 flex-1 border-t border-ops-border-subtle">
        {activeTab === 'chat' ? <ChatSidebar collapsed={collapsed} /> : null}
        {activeTab === 'workflow' ? (
          <WorkflowSidebar
            collapsed={collapsed}
            filter={workflowFilter}
            onFilterChange={setWorkflowFilter}
          />
        ) : null}
        {activeTab === 'reports' ? (
          <ReportSidebar
            collapsed={collapsed}
            filter={reportFilter}
            onFilterChange={setReportFilter}
          />
        ) : null}
      </div>
    </aside>
  );
}

function ChatSidebar({ collapsed }: { collapsed: boolean }) {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { data } = useAISessions();
  const deleteSession = useDeleteAISession();
  const selectedSessionID = searchParams.get('session');
  const sessions = [...(data ?? [])].sort((a, b) => b.updated_at.localeCompare(a.updated_at));

  async function handleDelete(sessionID: string, title: string) {
    if (!confirm(`删除会话「${title}」？此操作不可撤销。`)) return;
    try {
      await deleteSession.mutateAsync(sessionID);
      // 如果删的是当前选中会话，清掉 URL 中的 session 参数
      if (selectedSessionID === sessionID) {
        navigate('/?tab=chat');
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '删除会话失败');
    }
  }

  return (
    <div className="flex h-full flex-col">
      {!collapsed ? (
        <div className="p-3">
          <button
            type="button"
            className="w-full rounded-md bg-ops-accent px-3 py-2 text-sm font-medium text-ops-inverse hover:bg-ops-accent-hover"
            onClick={() => navigate('/?tab=chat')}
          >
            + 新会话
          </button>
          <div className="mt-4 text-2xs uppercase text-ops-tertiary">最近</div>
        </div>
      ) : null}
      <SidebarList>
        {sessions.map((session) => (
          <SidebarItem
            key={session.id}
            title={session.title}
            meta={new Date(session.updated_at).toLocaleString()}
            collapsed={collapsed}
            icon={<MessageSquare size={14} />}
            selected={selectedSessionID === session.id}
            onClick={() => navigate(`/?tab=chat&session=${session.id}`)}
            onDelete={() => handleDelete(session.id, session.title)}
          />
        ))}
      </SidebarList>
    </div>
  );
}

function WorkflowSidebar({
  collapsed,
  filter,
  onFilterChange,
}: {
  collapsed: boolean;
  filter: WorkflowFilter;
  onFilterChange: (value: WorkflowFilter) => void;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const { data: workflows } = useWorkflows();
  const { data: assembles } = useAssembles();
  const deleteWorkflow = useDeleteWorkflow();
  const deleteAssemble = useDeleteAssemble();
  const activeWorkflowID = location.pathname.match(/^\/workflows\/([^/]+)/)?.[1];
  const activeAssembleID = location.pathname.match(/^\/assembles\/([^/]+)/)?.[1];
  // "+ 新建" 按钮直接打开对应 Dialog，避免跳转到首页再点一次"新建工作流"。
  const [createWorkflowOpen, setCreateWorkflowOpen] = useState(false);
  const [createAssembleOpen, setCreateAssembleOpen] = useState(false);

  async function handleDeleteWorkflow(id: string, name: string) {
    if (!confirm(`删除工作流「${name}」？此操作不可撤销。`)) return;
    try {
      await deleteWorkflow.mutateAsync(id);
      if (activeWorkflowID === id) navigate('/?tab=workflow');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '删除工作流失败');
    }
  }

  async function handleDeleteAssemble(id: string, name: string) {
    if (!confirm(`删除集合「${name}」？此操作不可撤销。`)) return;
    try {
      await deleteAssemble.mutateAsync(id);
      if (activeAssembleID === id) navigate('/?tab=workflow');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '删除集合失败');
    }
  }

  // 按当前筛选直接打开对应 Dialog；按钮文案也随之变化，避免歧义。
  function handleCreate() {
    if (filter === 'workflow') setCreateWorkflowOpen(true);
    else setCreateAssembleOpen(true);
  }

  return (
    <div className="flex h-full flex-col">
      {!collapsed ? (
        <div className="space-y-3 p-3">
          <button
            type="button"
            className="w-full rounded-md bg-ops-accent px-3 py-2 text-sm font-medium text-ops-inverse hover:bg-ops-accent-hover"
            onClick={handleCreate}
          >
            + 新建{filter === 'workflow' ? '工作流' : '集合'}
          </button>
          <SidebarFilter
            value={filter}
            options={[
              { value: 'workflow', label: '工作流' },
              { value: 'assemble', label: '集合' },
            ]}
            onChange={onFilterChange}
          />
        </div>
      ) : null}
      <SidebarList>
        {filter === 'workflow'
          ? (workflows ?? []).map((workflow) => (
              <SidebarItem
                key={workflow.id}
                title={workflow.name}
                meta={workflow.description || workflow.id}
                collapsed={collapsed}
                icon={<GitBranch size={14} />}
                selected={activeWorkflowID === workflow.id}
                onClick={() => navigate(`/workflows/${workflow.id}`)}
                onDelete={() => handleDeleteWorkflow(workflow.id, workflow.name)}
              />
            ))
          : (assembles ?? []).map((assemble) => (
              <SidebarItem
                key={assemble.id}
                title={assemble.name}
                meta={assemble.description || assemble.id}
                collapsed={collapsed}
                icon={<Layers size={14} />}
                selected={activeAssembleID === assemble.id}
                onClick={() => navigate(`/assembles/${assemble.id}`)}
                onDelete={() => handleDeleteAssemble(assemble.id, assemble.name)}
              />
            ))}
      </SidebarList>
      <CreateWorkflowDialog open={createWorkflowOpen} onOpenChange={setCreateWorkflowOpen} />
      <CreateAssembleDialog open={createAssembleOpen} onOpenChange={setCreateAssembleOpen} />
    </div>
  );
}

function ReportSidebar({
  collapsed,
  filter,
  onFilterChange,
}: {
  collapsed: boolean;
  filter: ReportFilter;
  onFilterChange: (value: ReportFilter) => void;
}) {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { data } = useOpsDocs();
  const deleteDoc = useDeleteOpsDoc();
  const selectedDocID = searchParams.get('doc');
  const docs = useMemo(() => {
    const all = data ?? [];
    return filter === 'all' ? all : all.filter((doc) => doc.kind === filter);
  }, [data, filter]);

  async function handleDelete(id: string, title: string) {
    if (!confirm(`删除文档「${title}」？此操作不可撤销。`)) return;
    try {
      await deleteDoc.mutateAsync(id);
      if (selectedDocID === id) navigate('/?tab=reports');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '删除文档失败');
    }
  }

  return (
    <div className="flex h-full flex-col">
      {!collapsed ? (
        <div className="p-3">
          <SidebarFilter
            value={filter}
            options={[
              { value: 'all', label: '全部' },
              { value: 'inspection', label: '巡检' },
              { value: 'troubleshooting', label: '排障' },
              { value: 'architecture', label: '架构' },
            ]}
            onChange={onFilterChange}
          />
        </div>
      ) : null}
      <SidebarList>
        {docs.map((doc) => (
          <SidebarItem
            key={doc.id}
            title={doc.title}
            meta={`${kindLabel(doc.kind)} · ${new Date(doc.updated_at).toLocaleString()}`}
            collapsed={collapsed}
            icon={<FileText size={14} />}
            selected={selectedDocID === doc.id}
            onClick={() => navigate(`/?tab=reports&doc=${doc.id}`)}
            onDelete={() => handleDelete(doc.id, doc.title)}
          />
        ))}
      </SidebarList>
    </div>
  );
}

function getActiveSidebarTab(pathname: string, queryTab: string | null): SidebarTab {
  if (pathname.startsWith('/workflows/') || pathname.startsWith('/assembles/') || pathname.startsWith('/executions/')) {
    return 'workflow';
  }
  if (queryTab === 'workflow' || queryTab === 'reports') return queryTab;
  return 'chat';
}

function kindLabel(kind: OpsDocKind) {
  switch (kind) {
    case 'inspection':
      return '巡检';
    case 'troubleshooting':
      return '排障';
    case 'architecture':
      return '架构';
  }
}
