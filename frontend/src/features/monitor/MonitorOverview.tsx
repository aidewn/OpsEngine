// 监控首页：承载环境概览、分组详情和监控项详情的前端骨架。
import {
  AlertTriangle,
  ArrowLeft,
  CheckCircle2,
  ClipboardList,
  DatabaseZap,
  Pencil,
  FolderKanban,
  Plus,
  Power,
  PowerOff,
  RadioTower,
  Stethoscope,
  Trash2,
} from 'lucide-react';
import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useEnvironments } from '@/api/environments';
import {
  useCreateMonitorSource,
  useDeleteMonitorSource,
  useMonitorOverview,
  useRunPanelDiagnosis,
  useTestMonitorSource,
  useUpdateMonitorSource,
} from '@/api/monitor';
import { Button } from '@/components/ui/Button';
import { Dialog } from '@/components/ui/Dialog';
import { Input } from '@/components/ui/Input';
import { Label } from '@/components/ui/Label';
import { Select } from '@/components/ui/Select';
import { cn } from '@/lib/cn';
import { errorMessage } from '@/lib/error';
import { toast } from '@/lib/toast';
import { AcknowledgeButton } from './AcknowledgeButton';
import { CreateMonitorGroupDialog } from './CreateMonitorGroupDialog';
import { CreateMonitorPanelDialog } from './CreateMonitorPanelDialog';
import { MonitorConfigControl } from './MonitorConfigControl';
import { MonitorReportDialog } from './MonitorReportDialog';
import { PanelConditionsEditor } from './PanelConditionsEditor';
import { PanelRequirementsEditor } from './PanelRequirementsEditor';
import { RunTickButton } from './RunTickButton';
import type {
  Incident,
  MonitorGroup,
  MonitorOverviewData,
  MonitorPanel,
  MonitorReport,
  MonitorSource,
  MonitorStatus,
  PanelState,
} from '@/types/monitor';

// MonitorOverview 根据 query 参数切换概览、分组和监控项详情。
export function MonitorOverview() {
  const [searchParams, setSearchParams] = useSearchParams();
  const { data: environments, isLoading: environmentsLoading } = useEnvironments();
  const selectedEnvID = searchParams.get('env') ?? environments?.[0]?.id ?? '';
  const selectedGroupID = searchParams.get('group') ?? '';
  const selectedPanelID = searchParams.get('panel') ?? '';
  const selectedEnv = environments?.find((env) => env.id === selectedEnvID);
  const { data: overview } = useMonitorOverview(selectedEnvID);

  const selectedGroup = overview?.groups.find((group) => group.id === selectedGroupID);
  const selectedPanel = overview?.panels.find((panel) => panel.id === selectedPanelID);
  const selectedPanelState = overview?.states.find((state) => state.panel_id === selectedPanelID);

  function updateMonitorParams(patch: Record<string, string | null>) {
    const next = new URLSearchParams(searchParams);
    next.set('tab', 'monitor');
    for (const [key, value] of Object.entries(patch)) {
      if (value) next.set(key, value);
      else next.delete(key);
    }
    setSearchParams(next);
  }

  function handleEnvironmentChange(environmentID: string) {
    updateMonitorParams({ env: environmentID, group: null, panel: null });
  }

  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-5">
      <MonitorHeader
        environmentName={selectedEnv?.name}
        environments={environments ?? []}
        environmentsLoading={environmentsLoading}
        selectedEnvID={selectedEnvID}
        onEnvironmentChange={handleEnvironmentChange}
      />

      {overview && selectedPanel ? (
        <PanelDetail
          overview={overview}
          panel={selectedPanel}
          state={selectedPanelState}
          onBack={() => updateMonitorParams({ panel: null })}
          onOpenGroup={(groupID) => updateMonitorParams({ group: groupID, panel: null })}
        />
      ) : overview && selectedGroup ? (
        <GroupDetail
          overview={overview}
          group={selectedGroup}
          onBack={() => updateMonitorParams({ group: null, panel: null })}
          onOpenPanel={(panelID) => updateMonitorParams({ panel: panelID })}
        />
      ) : overview ? (
        <EnvironmentOverview
          overview={overview}
          onOpenGroup={(groupID) => updateMonitorParams({ group: groupID, panel: null })}
          onOpenPanel={(panelID) => updateMonitorParams({ panel: panelID })}
        />
      ) : (
        <div className="rounded-lg border border-ops-border-subtle bg-ops-surface px-4 py-8 text-sm text-ops-secondary">
          正在加载监控概览...
        </div>
      )}
    </div>
  );
}

function MonitorHeader({
  environmentName,
  environments,
  environmentsLoading,
  selectedEnvID,
  onEnvironmentChange,
}: {
  environmentName: string | undefined;
  environments: Array<{ id: string; name: string }>;
  environmentsLoading: boolean;
  selectedEnvID: string;
  onEnvironmentChange: (environmentID: string) => void;
}) {
  return (
    <header className="flex flex-wrap items-start justify-between gap-4 border-b border-ops-border-subtle pb-5">
      <div>
        <div className="flex items-center gap-2 text-sm text-ops-secondary">
          <RadioTower size={16} />
          <span>监控</span>
        </div>
        <h1 className="mt-2 text-2xl font-semibold text-ops-primary">
          {environmentName ?? '监控概览'}
        </h1>
        <p className="mt-1 max-w-2xl text-sm text-ops-secondary">
          先选择环境，再按分组进入具体监控项；概览页聚合异常项、诊断中项和异常历史。
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {environments.slice(0, 4).map((env) => (
          <Button
            key={env.id}
            size="sm"
            variant={env.id === selectedEnvID ? 'accent' : 'secondary'}
            onClick={() => onEnvironmentChange(env.id)}
          >
            {env.name}
          </Button>
        ))}
        {!environmentsLoading && environments.length === 0 ? (
          <Button size="sm" variant="secondary">
            暂无环境
          </Button>
        ) : null}
      </div>
    </header>
  );
}

function EnvironmentOverview({
  overview,
  onOpenGroup,
  onOpenPanel,
}: {
  overview: MonitorOverviewData;
  onOpenGroup: (groupID: string) => void;
  onOpenPanel: (panelID: string) => void;
}) {
  const stats = useMemo(() => buildStats(overview.states), [overview.states]);
  const rows = buildPanelRows(overview);
  const problemRows = rows.filter((row) => row.state.status === 'abnormal');
  const diagnosingRows = rows.filter((row) => row.state.status === 'diagnosing');
  const historyRows = rows.filter((row) => row.state.status === 'history');
  const [groupDialogOpen, setGroupDialogOpen] = useState(false);
  const [panelDialogOpen, setPanelDialogOpen] = useState(false);
  const [sourceDialogOpen, setSourceDialogOpen] = useState(false);
  const [editingSource, setEditingSource] = useState<MonitorSource | undefined>(undefined);
  const [reportOpen, setReportOpen] = useState(false);
  const [viewReport, setViewReport] = useState<MonitorReport | undefined>(undefined);

  return (
    <>
      <section className="flex flex-wrap items-center justify-end gap-2">
        <MonitorConfigControl environmentID={overview.environment_id} />
        <RunTickButton environmentID={overview.environment_id} />
        <Button size="sm" variant="secondary" onClick={() => setGroupDialogOpen(true)}>
          <Plus size={14} />
          新建分组
        </Button>
        <Button size="sm" onClick={() => setPanelDialogOpen(true)}>
          <Plus size={14} />
          新建监控项
        </Button>
      </section>

      <section className="grid gap-3 md:grid-cols-4">
        <MetricTile
          label="异常项"
          value={stats.abnormal}
          tone="danger"
          icon={<AlertTriangle size={18} />}
        />
        <MetricTile
          label="诊断中"
          value={stats.diagnosing}
          tone="info"
          icon={<Stethoscope size={18} />}
        />
        <MetricTile
          label="有异常历史"
          value={stats.history}
          tone="warn"
          icon={<ClipboardList size={18} />}
        />
        <MetricTile
          label="正常项"
          value={stats.normal}
          tone="success"
          icon={<CheckCircle2 size={18} />}
        />
      </section>

      <section className="grid min-h-0 gap-4 xl:grid-cols-[minmax(0,1.5fr)_minmax(320px,0.8fr)]">
        <div className="space-y-4">
          <OverviewSection title="当前有问题" count={problemRows.length}>
            {problemRows.map((row) => (
              <MonitorItemRow key={row.panel.id} row={row} onOpenPanel={onOpenPanel} />
            ))}
          </OverviewSection>
          <OverviewSection title="诊断中" count={diagnosingRows.length}>
            {diagnosingRows.map((row) => (
              <MonitorItemRow key={row.panel.id} row={row} onOpenPanel={onOpenPanel} />
            ))}
          </OverviewSection>
          <OverviewSection title="正常（有异常历史）" count={historyRows.length}>
            {historyRows.map((row) => (
              <MonitorItemRow
                key={row.panel.id}
                row={row}
                onOpenPanel={onOpenPanel}
                action={<AcknowledgeButton panelID={row.panel.id} />}
              />
            ))}
          </OverviewSection>
        </div>

        <aside className="space-y-4">
          <div className="rounded-lg border border-ops-border-subtle bg-ops-surface p-4">
            <div className="flex items-center gap-2 text-sm font-medium text-ops-primary">
              <FolderKanban size={16} />
              <span>分组</span>
            </div>
            <div className="mt-3 space-y-2">
              {overview.groups.map((group) => {
                const count = overview.panels.filter((panel) => panel.group_id === group.id).length;
                return (
                  <button
                    key={group.id}
                    type="button"
                    className="flex w-full items-center justify-between rounded-md border border-ops-border-subtle bg-ops-elevated px-3 py-2 text-left text-sm text-ops-primary hover:border-ops-border-strong"
                    onClick={() => onOpenGroup(group.id)}
                  >
                    <span>{group.name}</span>
                    <span className="text-xs text-ops-tertiary">{count} 项</span>
                  </button>
                );
              })}
            </div>
          </div>

          <MonitorSourcesCard
            sources={overview.sources ?? []}
            onCreate={() => {
              setEditingSource(undefined);
              setSourceDialogOpen(true);
            }}
            onEdit={(source) => {
              setEditingSource(source);
              setSourceDialogOpen(true);
            }}
          />

          <div className="rounded-lg border border-ops-border-subtle bg-ops-surface p-4">
            <div className="text-sm font-medium text-ops-primary">最近报告</div>
            <div className="mt-3 space-y-3 text-sm">
              {overview.reports.length === 0 ? (
                <div className="text-sm text-ops-tertiary">暂无报告</div>
              ) : (
                overview.reports.map((report) => (
                  <ReportLink
                    key={report.id}
                    title={report.title}
                    meta={new Date(report.created_at).toLocaleString()}
                    onClick={() => {
                      setViewReport(report);
                      setReportOpen(true);
                    }}
                  />
                ))
              )}
            </div>
          </div>
        </aside>
      </section>

      <CreateMonitorGroupDialog
        open={groupDialogOpen}
        onOpenChange={setGroupDialogOpen}
        environmentID={overview.environment_id}
      />
      <CreateMonitorPanelDialog
        open={panelDialogOpen}
        onOpenChange={setPanelDialogOpen}
        environmentID={overview.environment_id}
        groups={overview.groups}
      />
      <MonitorSourceDialog
        open={sourceDialogOpen}
        onOpenChange={setSourceDialogOpen}
        environmentID={overview.environment_id}
        source={editingSource}
      />
      <MonitorReportDialog report={viewReport} open={reportOpen} onOpenChange={setReportOpen} />
    </>
  );
}

function GroupDetail({
  overview,
  group,
  onBack,
  onOpenPanel,
}: {
  overview: MonitorOverviewData;
  group: MonitorGroup;
  onBack: () => void;
  onOpenPanel: (panelID: string) => void;
}) {
  const rows = buildPanelRows(overview).filter((row) => row.group.id === group.id);
  const [panelDialogOpen, setPanelDialogOpen] = useState(false);

  return (
    <section className="space-y-4">
      <DetailToolbar
        title={group.name}
        subtitle={group.description}
        onBack={onBack}
        actions={
          <Button size="sm" onClick={() => setPanelDialogOpen(true)}>
            <Plus size={14} />
            新建监控项
          </Button>
        }
      />
      <OverviewSection title="监控项" count={rows.length}>
        {rows.map((row) => (
          <MonitorItemRow key={row.panel.id} row={row} onOpenPanel={onOpenPanel} />
        ))}
      </OverviewSection>

      <CreateMonitorPanelDialog
        open={panelDialogOpen}
        onOpenChange={setPanelDialogOpen}
        environmentID={overview.environment_id}
        groups={overview.groups}
        defaultGroupID={group.id}
      />
    </section>
  );
}

function PanelDetail({
  overview,
  panel,
  state,
  onBack,
  onOpenGroup,
}: {
  overview: MonitorOverviewData;
  panel: MonitorPanel;
  state: PanelState | undefined;
  onBack: () => void;
  onOpenGroup: (groupID: string) => void;
}) {
  const group = overview.groups.find((item) => item.id === panel.group_id);
  const incident = overview.incidents.find((item) => item.id === state?.current_incident_id);
  const report = overview.reports.find((item) => item.id === state?.last_report_id);
  const status = statusMeta(state?.status ?? 'normal');

  const diagnose = useRunPanelDiagnosis();
  const [reportOpen, setReportOpen] = useState(false);
  const [viewReport, setViewReport] = useState<MonitorReport | undefined>(undefined);

  function openReport(r: MonitorReport) {
    setViewReport(r);
    setReportOpen(true);
  }

  function handleDiagnose() {
    diagnose.mutate(panel.id, {
      onSuccess: () => toast.success('诊断已开始，完成后报告会出现在「最近报告」'),
      onError: (err) => toast.error(errorMessage(err, '诊断失败')),
    });
  }

  // 有未结异常且当前不在诊断中时才可触发。
  const diagnosing = state?.status === 'diagnosing';
  const canDiagnose = !!state?.current_incident_id && !diagnosing;
  const isHistory = state?.status === 'history';

  return (
    <section className="space-y-4">
      <DetailToolbar title={panel.name} subtitle={panel.description} onBack={onBack} />
      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="space-y-4">
          <div className="rounded-lg border border-ops-border-subtle bg-ops-surface p-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <div className="text-xs text-ops-tertiary">当前状态</div>
                <div className={cn('mt-1 text-lg font-semibold', status.className)}>
                  {status.label}
                </div>
              </div>
              <div className="flex items-center gap-2">
                {isHistory ? <AcknowledgeButton panelID={panel.id} /> : null}
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={!canDiagnose || diagnose.isPending}
                  title={canDiagnose ? '基于当前异常生成诊断报告' : '当前无异常或诊断进行中'}
                  onClick={handleDiagnose}
                >
                  {diagnosing || diagnose.isPending ? '诊断中...' : '启动诊断'}
                </Button>
              </div>
            </div>
            <div className="mt-4 text-sm text-ops-secondary">
              {state?.summary ?? '暂无状态'}
            </div>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <FlowCard title="监控流程" description={panel.monitor_flow_summary} />
            <FlowCard title="排查流程" description={panel.troubleshoot_flow_summary} />
          </div>

          <PanelRequirementsEditor panel={panel} />
          <PanelConditionsEditor panel={panel} />
        </div>

        <aside className="space-y-4">
          <div className="rounded-lg border border-ops-border-subtle bg-ops-surface p-4">
            <div className="text-sm font-medium text-ops-primary">归属</div>
            <button
              type="button"
              className="mt-3 flex w-full items-center justify-between rounded-md border border-ops-border-subtle bg-ops-elevated px-3 py-2 text-left text-sm text-ops-primary hover:border-ops-border-strong"
              onClick={() => group && onOpenGroup(group.id)}
            >
              <span>{group?.name ?? '未分组'}</span>
              <span className="text-xs text-ops-tertiary">查看分组</span>
            </button>
          </div>

          <IncidentPanel incident={incident} />

          <div className="rounded-lg border border-ops-border-subtle bg-ops-surface p-4">
            <div className="text-sm font-medium text-ops-primary">最近报告</div>
            {report ? (
              <div className="mt-3 space-y-2">
                <div className="text-sm text-ops-primary">{report.title}</div>
                <div className="text-xs text-ops-secondary">{report.summary}</div>
                <Button size="sm" variant="secondary" onClick={() => openReport(report)}>
                  查看报告
                </Button>
              </div>
            ) : (
              <div className="mt-3 text-sm text-ops-tertiary">暂无报告</div>
            )}
          </div>
        </aside>
      </div>

      <MonitorReportDialog report={viewReport} open={reportOpen} onOpenChange={setReportOpen} />
    </section>
  );
}

// IncidentPanel 展示当前异常事件的状态、起止时间与触发证据。
function IncidentPanel({ incident }: { incident: Incident | undefined }) {
  return (
    <div className="rounded-lg border border-ops-border-subtle bg-ops-surface p-4">
      <div className="text-sm font-medium text-ops-primary">当前异常</div>
      {incident ? (
        <div className="mt-3 space-y-2">
          <div className="text-xs text-ops-tertiary">
            {incidentStatusLabel(incident.status)} · 开始于{' '}
            {new Date(incident.started_at).toLocaleString()}
          </div>
          {incident.evidence.map((item) => (
            <div
              key={item}
              className="rounded-md bg-ops-elevated px-3 py-2 text-sm text-ops-secondary"
            >
              {item}
            </div>
          ))}
        </div>
      ) : (
        <div className="mt-3 text-sm text-ops-tertiary">暂无打开的异常事件</div>
      )}
    </div>
  );
}

// incidentStatusLabel 把异常事件状态转为展示文案。
function incidentStatusLabel(status: Incident['status']): string {
  switch (status) {
    case 'open':
      return '未处理';
    case 'diagnosing':
      return '诊断中';
    case 'resolved':
      return '已解决';
  }
}

function DetailToolbar({
  title,
  subtitle,
  onBack,
  actions,
}: {
  title: string;
  subtitle: string;
  onBack: () => void;
  actions?: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-start gap-3 rounded-lg border border-ops-border-subtle bg-ops-surface p-4">
      <Button
        size="sm"
        variant="ghost"
        title="返回"
        aria-label="返回"
        onClick={onBack}
      >
        <ArrowLeft size={14} />
      </Button>
      <div className="min-w-0 flex-1">
        <h2 className="text-lg font-semibold text-ops-primary">{title}</h2>
        <p className="mt-1 text-sm text-ops-secondary">{subtitle}</p>
      </div>
      {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
    </div>
  );
}

function MetricTile({
  label,
  value,
  tone,
  icon,
}: {
  label: string;
  value: number;
  tone: 'danger' | 'info' | 'warn' | 'success';
  icon: ReactNode;
}) {
  const toneClass = {
    danger: 'text-ops-danger',
    info: 'text-ops-info',
    warn: 'text-ops-warning',
    success: 'text-ops-success',
  }[tone];

  return (
    <div className="rounded-lg border border-ops-border-subtle bg-ops-surface p-4">
      <div className={cn('flex items-center gap-2 text-sm', toneClass)}>
        {icon}
        <span>{label}</span>
      </div>
      <div className="mt-3 text-3xl font-semibold text-ops-primary">{value}</div>
    </div>
  );
}

function OverviewSection({
  title,
  count,
  children,
}: {
  title: string;
  count: number;
  children: ReactNode;
}) {
  return (
    <section className="rounded-lg border border-ops-border-subtle bg-ops-surface">
      <div className="flex items-center justify-between border-b border-ops-border-subtle px-4 py-3">
        <h2 className="text-sm font-medium text-ops-primary">{title}</h2>
        <span className="text-xs text-ops-tertiary">{count} 项</span>
      </div>
      <div className="divide-y divide-ops-border-subtle">{children}</div>
    </section>
  );
}

type PanelRow = {
  panel: MonitorPanel;
  group: MonitorGroup;
  state: PanelState;
};

function MonitorItemRow({
  row,
  onOpenPanel,
  action,
}: {
  row: PanelRow;
  onOpenPanel: (panelID: string) => void;
  action?: ReactNode;
}) {
  const status = statusMeta(row.state.status);
  const checkedAt = row.state.last_checked_at
    ? new Date(row.state.last_checked_at).toLocaleTimeString()
    : '';
  return (
    <div className="flex items-center gap-2 px-4 py-3 hover:bg-ops-elevated">
      <button
        type="button"
        className="grid flex-1 gap-3 text-left md:grid-cols-[minmax(0,1fr)_150px_100px]"
        onClick={() => onOpenPanel(row.panel.id)}
      >
        <div className="min-w-0">
          <div className="truncate text-sm font-medium text-ops-primary">{row.panel.name}</div>
          <div className="mt-1 truncate text-xs text-ops-secondary">
            {row.group.name} · {row.state.summary}
          </div>
        </div>
        <div className={cn('text-sm font-medium', status.className)}>{status.label}</div>
        <div className="text-xs text-ops-tertiary">{checkedAt}</div>
      </button>
      {action ? <div className="shrink-0">{action}</div> : null}
    </div>
  );
}

function FlowCard({ title, description }: { title: string; description: string }) {
  return (
    <div className="rounded-lg border border-ops-border-subtle bg-ops-surface p-4">
      <div className="flex items-center gap-2 text-sm font-medium text-ops-primary">
        <DatabaseZap size={16} />
        <span>{title}</span>
      </div>
      <p className="mt-3 text-sm text-ops-secondary">{description}</p>
    </div>
  );
}

function MonitorSourcesCard({
  sources,
  onCreate,
  onEdit,
}: {
  sources: MonitorSource[];
  onCreate: () => void;
  onEdit: (source: MonitorSource) => void;
}) {
  const update = useUpdateMonitorSource();
  const remove = useDeleteMonitorSource();
  const busy = update.isPending || remove.isPending;

  async function toggleSource(source: MonitorSource) {
    try {
      await update.mutateAsync({ ...source, enabled: !source.enabled });
      toast.success(source.enabled ? '监控源已禁用' : '监控源已启用');
    } catch (err) {
      toast.error(errorMessage(err, '更新监控源失败'));
    }
  }

  async function deleteSource(source: MonitorSource) {
    if (!window.confirm(`删除监控源「${source.name}」？`)) return;
    try {
      await remove.mutateAsync({ id: source.id, environmentID: source.environment_id });
      toast.success('监控源已删除');
    } catch (err) {
      toast.error(errorMessage(err, '删除监控源失败'));
    }
  }

  return (
    <div className="rounded-lg border border-ops-border-subtle bg-ops-surface p-4">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-sm font-medium text-ops-primary">
          <DatabaseZap size={16} />
          <span>监控源</span>
        </div>
        <Button size="sm" variant="secondary" onClick={onCreate}>
          <Plus size={14} />
          新增
        </Button>
      </div>
      <div className="mt-3 space-y-2">
        {sources.length === 0 ? (
          <div className="text-sm text-ops-tertiary">暂无监控源</div>
        ) : (
          sources.map((source) => {
            const builtin = source.id === 'builtin';
            return (
              <div
                key={source.id}
                className="rounded-md border border-ops-border-subtle bg-ops-elevated px-3 py-2"
              >
                <div className="flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <div className="truncate text-sm text-ops-primary">{source.name}</div>
                    <div className="mt-1 truncate text-xs text-ops-tertiary">
                      {source.kind}
                      {source.config?.endpoint ? ` · ${String(source.config.endpoint)}` : ''}
                    </div>
                  </div>
                  <span
                    className={cn(
                      'shrink-0 text-xs',
                      source.enabled ? 'text-ops-success' : 'text-ops-tertiary',
                    )}
                  >
                    {source.enabled ? '启用' : '禁用'}
                  </span>
                </div>
                {!builtin ? (
                  <div className="mt-2 flex justify-end gap-1">
                    <Button
                      size="sm"
                      variant="ghost"
                      title="编辑"
                      aria-label="编辑监控源"
                      disabled={busy}
                      onClick={() => onEdit(source)}
                    >
                      <Pencil size={14} />
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      title={source.enabled ? '禁用' : '启用'}
                      aria-label={source.enabled ? '禁用监控源' : '启用监控源'}
                      disabled={busy}
                      onClick={() => toggleSource(source)}
                    >
                      {source.enabled ? <PowerOff size={14} /> : <Power size={14} />}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      title="删除"
                      aria-label="删除监控源"
                      disabled={busy}
                      onClick={() => deleteSource(source)}
                    >
                      <Trash2 size={14} />
                    </Button>
                  </div>
                ) : null}
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}

function MonitorSourceDialog({
  open,
  onOpenChange,
  environmentID,
  source,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  environmentID: string;
  source?: MonitorSource;
}) {
  const create = useCreateMonitorSource();
  const update = useUpdateMonitorSource();
  const test = useTestMonitorSource();
  const editing = !!source?.id;
  const [name, setName] = useState(source?.name ?? '生产 Prometheus');
  const [endpoint, setEndpoint] = useState('');
  const [authType, setAuthType] = useState('none');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [token, setToken] = useState('');
  const [timeout, setTimeoutValue] = useState(10);
  const busy = create.isPending || update.isPending || test.isPending;

  useEffect(() => {
    if (!open) return;
    const config = source?.config ?? {};
    setName(source?.name ?? '生产 Prometheus');
    setEndpoint(String(config.endpoint ?? ''));
    setAuthType(String(config.auth_type ?? 'none'));
    setUsername(String(config.username ?? ''));
    setPassword(String(config.password ?? ''));
    setToken(String(config.token ?? ''));
    setTimeoutValue(Number(config.timeout_seconds ?? 10) || 10);
  }, [open, source]);

  function buildSource(): MonitorSource {
    return {
      id: '',
      environment_id: environmentID,
      name,
      kind: 'prometheus',
      enabled: true,
      config: {
        endpoint,
        auth_type: authType,
        username,
        password,
        token,
        timeout_seconds: timeout,
      },
    };
  }

  async function handleTest() {
    try {
      await test.mutateAsync(buildSource());
      toast.success('监控源连接正常');
    } catch (err) {
      toast.error(errorMessage(err, '监控源连接失败'));
    }
  }

  async function handleCreate() {
    try {
      if (editing && source) {
        await update.mutateAsync({
          ...source,
          name,
          kind: 'prometheus',
          config: buildSource().config,
        });
        toast.success('监控源已更新');
      } else {
        await create.mutateAsync({
          environmentID,
          name,
          kind: 'prometheus',
          config: buildSource().config,
        });
        toast.success('监控源已创建');
      }
      onOpenChange(false);
    } catch (err) {
      toast.error(errorMessage(err, editing ? '更新监控源失败' : '创建监控源失败'));
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={editing ? '编辑监控源' : '新增监控源'}
      description="当前先支持 Prometheus 即时查询。"
      footer={
        <>
          <Button variant="secondary" onClick={handleTest} disabled={busy}>
            {test.isPending ? '测试中...' : '测试连接'}
          </Button>
          <Button onClick={handleCreate} disabled={busy}>
            {create.isPending || update.isPending ? '保存中...' : '保存'}
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        <div className="space-y-1">
          <Label>名称</Label>
          <Input value={name} disabled={busy} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="space-y-1">
          <Label>Endpoint</Label>
          <Input
            value={endpoint}
            placeholder="http://prometheus:9090"
            disabled={busy}
            onChange={(e) => setEndpoint(e.target.value)}
          />
        </div>
        <div className="grid gap-3 md:grid-cols-2">
          <div className="space-y-1">
            <Label>认证方式</Label>
            <Select value={authType} disabled={busy} onChange={(e) => setAuthType(e.target.value)}>
              <option value="none">无</option>
              <option value="basic">Basic</option>
              <option value="bearer">Bearer Token</option>
            </Select>
          </div>
          <div className="space-y-1">
            <Label>超时（秒）</Label>
            <Input
              type="number"
              min={1}
              value={String(timeout)}
              disabled={busy}
              onChange={(e) => setTimeoutValue(Number(e.target.value) || 10)}
            />
          </div>
        </div>
        {authType === 'basic' ? (
          <div className="grid gap-3 md:grid-cols-2">
            <div className="space-y-1">
              <Label>用户名</Label>
              <Input value={username} disabled={busy} onChange={(e) => setUsername(e.target.value)} />
            </div>
            <div className="space-y-1">
              <Label>密码</Label>
              <Input
                type="password"
                value={password}
                disabled={busy}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
          </div>
        ) : null}
        {authType === 'bearer' ? (
          <div className="space-y-1">
            <Label>Token</Label>
            <Input
              type="password"
              value={token}
              disabled={busy}
              onChange={(e) => setToken(e.target.value)}
            />
          </div>
        ) : null}
      </div>
    </Dialog>
  );
}

function ReportLink({
  title,
  meta,
  onClick,
}: {
  title: string;
  meta: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className="block w-full rounded-md border border-ops-border-subtle bg-ops-elevated px-3 py-2 text-left hover:border-ops-border-strong"
      onClick={onClick}
    >
      <div className="truncate text-sm text-ops-primary">{title}</div>
      <div className="mt-1 text-xs text-ops-tertiary">{meta}</div>
    </button>
  );
}

function buildStats(states: PanelState[]) {
  return {
    abnormal: states.filter((state) => state.status === 'abnormal').length,
    diagnosing: states.filter((state) => state.status === 'diagnosing').length,
    history: states.filter((state) => state.status === 'history').length,
    normal: states.filter((state) => state.status === 'normal').length,
  };
}

function buildPanelRows(overview: MonitorOverviewData): PanelRow[] {
  return overview.panels.flatMap((panel) => {
    const group = overview.groups.find((item) => item.id === panel.group_id);
    const state = overview.states.find((item) => item.panel_id === panel.id);
    if (!group || !state) return [];
    return [{ panel, group, state }];
  });
}

function statusMeta(status: MonitorStatus) {
  switch (status) {
    case 'abnormal':
      return { label: '有异常', className: 'text-ops-danger' };
    case 'diagnosing':
      return { label: '诊断中', className: 'text-ops-info' };
    case 'history':
      return { label: '正常（有异常历史）', className: 'text-ops-warning' };
    case 'normal':
      return { label: '正常', className: 'text-ops-success' };
  }
}
