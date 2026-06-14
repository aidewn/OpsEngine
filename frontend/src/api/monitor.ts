// 监控 API hooks（TanStack Query + Wails 绑定）。
// Phase 1 消费概览查询 + 分组/监控项创建；编辑/删除待对应 UI 落地时再补。
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';
import {
  AcknowledgePanelHistory,
  CreateMonitorGroup,
  CreateMonitorPanel,
  GetMonitorConfig,
  GetMonitorOverview,
  RunMonitorTick,
  RunPanelDiagnosis,
  SetMonitorConfig,
  UpdateMonitorPanel,
} from '@wails/go/main/App';
import type { MonitorConfig, MonitorOverviewData, MonitorPanel } from '@/types/monitor';

const KEY = {
  overview: (environmentID: string) => ['monitor', 'overview', environmentID] as const,
  config: (environmentID: string) => ['monitor', 'config', environmentID] as const,
};

// useMonitorOverview 返回当前环境的监控概览数据。
// 采集由后台调度器自动进行，这里周期性重拉，让界面被动反映最新状态。
export function useMonitorOverview(
  environmentID: string | undefined,
): UseQueryResult<MonitorOverviewData> {
  return useQuery({
    queryKey: KEY.overview(environmentID ?? ''),
    queryFn: () => GetMonitorOverview(environmentID!) as Promise<MonitorOverviewData>,
    enabled: !!environmentID,
    refetchInterval: 15000,
  });
}

// useMonitorConfig 返回环境的监控调度配置。
export function useMonitorConfig(
  environmentID: string | undefined,
): UseQueryResult<MonitorConfig> {
  return useQuery({
    queryKey: KEY.config(environmentID ?? ''),
    queryFn: () => GetMonitorConfig(environmentID!) as Promise<MonitorConfig>,
    enabled: !!environmentID,
  });
}

// 设置监控调度配置入参。
export interface SetMonitorConfigInput {
  environmentID: string;
  enabled: boolean;
  intervalSeconds: number;
}

// useSetMonitorConfig 开关自动监控 / 调整采集间隔，成功后回写配置缓存。
export function useSetMonitorConfig(): UseMutationResult<
  MonitorConfig,
  Error,
  SetMonitorConfigInput
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input) =>
      SetMonitorConfig(input.environmentID, input.enabled, input.intervalSeconds) as Promise<MonitorConfig>,
    onSuccess: (config, input) => {
      qc.setQueryData(KEY.config(input.environmentID), config);
    },
  });
}

// 创建分组入参。
export interface CreateMonitorGroupInput {
  environmentID: string;
  name: string;
  description?: string;
}

// useCreateMonitorGroup 在指定环境下创建分组，成功后失效该环境概览缓存。
export function useCreateMonitorGroup(): UseMutationResult<
  string,
  Error,
  CreateMonitorGroupInput
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input) =>
      CreateMonitorGroup(input.environmentID, input.name, input.description ?? ''),
    onSuccess: (_id, input) => {
      qc.invalidateQueries({ queryKey: KEY.overview(input.environmentID) });
    },
  });
}

// 创建监控项入参。
export interface CreateMonitorPanelInput {
  environmentID: string;
  groupID: string;
  name: string;
  description?: string;
}

// useCreateMonitorPanel 在指定环境的分组下创建监控项，成功后失效该环境概览缓存。
export function useCreateMonitorPanel(): UseMutationResult<
  string,
  Error,
  CreateMonitorPanelInput
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input) =>
      CreateMonitorPanel(
        input.environmentID,
        input.groupID,
        input.name,
        input.description ?? '',
      ),
    onSuccess: (_id, input) => {
      qc.invalidateQueries({ queryKey: KEY.overview(input.environmentID) });
    },
  });
}

// useUpdateMonitorPanel 整体覆盖更新监控项（含数据需求），成功后失效该环境概览缓存。
export function useUpdateMonitorPanel(): UseMutationResult<void, Error, MonitorPanel> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (panel) => UpdateMonitorPanel(panel as never),
    onSuccess: (_data, panel) => {
      qc.invalidateQueries({ queryKey: KEY.overview(panel.environment_id) });
    },
  });
}

// useRunPanelDiagnosis 异步触发监控项诊断（Troubleshoot Flow）：仅启动，后台执行。
// 成功后失效概览缓存以立刻显示「诊断中」；报告完成后由概览轮询自动出现。
export function useRunPanelDiagnosis(): UseMutationResult<void, Error, string> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (panelID) => RunPanelDiagnosis(panelID),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['monitor', 'overview'] });
    },
  });
}

// useAcknowledgePanelHistory 确认监控项异常已恢复（history → normal），成功后刷新概览。
export function useAcknowledgePanelHistory(): UseMutationResult<void, Error, string> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (panelID) => AcknowledgePanelHistory(panelID),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['monitor', 'overview'] });
    },
  });
}

// useRunMonitorTick 手动「立即检查一次」：跑一轮完整监控并用返回的概览直接刷新缓存。
export function useRunMonitorTick(): UseMutationResult<MonitorOverviewData, Error, string> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (environmentID) =>
      RunMonitorTick(environmentID) as Promise<MonitorOverviewData>,
    onSuccess: (overview, environmentID) => {
      qc.setQueryData(KEY.overview(environmentID), overview);
    },
  });
}
