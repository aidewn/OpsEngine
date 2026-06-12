// AI 相关 API hooks，封装 Wails 后端方法。

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from "@tanstack/react-query";
import {
  ApplyAIPendingDraft,
  CreateAISession,
  ClearAISessionActiveArtifact,
  DeleteAISession,
  DiscardAIPendingDraft,
  GetAISession,
  GetAISettings,
  ListAISessions,
  SetAISessionActiveArtifact,
  StartAIAssistant,
  TestAISettings,
  UpdateAISessionContext,
  UpdateAISessionTitle,
  UpdateAISettings,
} from "@wails/go/main/App";
import type { AIAssistantRequest, AISession, AISettings } from "@/types/ai";
import type { WorkflowDef } from "@/types/workflow";

// KEY 统一管理 AI 缓存键。
const KEY = {
  settings: ["ai", "settings"] as const,
  sessions: ["ai", "sessions"] as const,
  session: (id: string) => ["ai", "session", id] as const,
};

// useAISettings 读取 AI 设置。
export function useAISettings(): UseQueryResult<AISettings> {
  return useQuery({
    queryKey: KEY.settings,
    queryFn: () => GetAISettings() as Promise<AISettings>,
  });
}

// useUpdateAISettings 保存 AI 设置。
export function useUpdateAISettings(): UseMutationResult<
  void,
  Error,
  AISettings
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (settings) => UpdateAISettings(settings as never),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: KEY.settings });
    },
  });
}

// useTestAISettings 验证当前保存的 AI 设置。
export function useTestAISettings(): UseMutationResult<string, Error, void> {
  return useMutation({
    mutationFn: () => TestAISettings(),
  });
}

// useAISessions 列出所有 AI 会话。
export function useAISessions(): UseQueryResult<AISession[]> {
  return useQuery({
    queryKey: KEY.sessions,
    queryFn: () => ListAISessions() as Promise<AISession[]>,
  });
}

// useAISession 加载单个会话。null/undefined id 时禁用查询。
export function useAISession(
  id: string | null | undefined,
): UseQueryResult<AISession> {
  return useQuery({
    queryKey: KEY.session(id ?? ""),
    queryFn: () => GetAISession(id as string) as Promise<AISession>,
    enabled: !!id,
  });
}

// useCreateAISession 创建新会话。
export function useCreateAISession(): UseMutationResult<
  AISession,
  Error,
  { environment_id: string; config_id: string; title?: string }
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ environment_id, config_id, title }) =>
      CreateAISession(
        environment_id,
        config_id,
        title ?? "",
      ) as Promise<AISession>,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: KEY.sessions });
    },
  });
}

// useUpdateAISessionTitle 重命名会话。
export function useUpdateAISessionTitle(): UseMutationResult<
  void,
  Error,
  { id: string; title: string }
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, title }) => UpdateAISessionTitle(id, title),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: KEY.sessions });
      qc.invalidateQueries({ queryKey: KEY.session(vars.id) });
    },
  });
}

// useUpdateAISessionContext 更新已有会话的环境/SSH 上下文。
export function useUpdateAISessionContext(): UseMutationResult<
  void,
  Error,
  { id: string; environment_id: string; config_id: string }
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, environment_id, config_id }) =>
      UpdateAISessionContext(id, environment_id, config_id),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: KEY.sessions });
      qc.invalidateQueries({ queryKey: KEY.session(vars.id) });
    },
  });
}

// useDeleteAISession 删除会话。
export function useDeleteAISession(): UseMutationResult<void, Error, string> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id) => DeleteAISession(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: KEY.sessions });
    },
  });
}

// useSetAISessionActiveArtifact 进入 artifact 编辑模式。
export function useSetAISessionActiveArtifact(): UseMutationResult<
  void,
  Error,
  {
    sessionID: string;
    artifactType: "workflow" | "assemble";
    artifactID: string;
    artifactName: string;
  }
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ sessionID, artifactType, artifactID, artifactName }) =>
      SetAISessionActiveArtifact(
        sessionID,
        artifactType,
        artifactID,
        artifactName,
      ),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: KEY.sessions });
      qc.invalidateQueries({ queryKey: KEY.session(vars.sessionID) });
    },
  });
}

// useClearAISessionActiveArtifact 退出 artifact 编辑模式。
export function useClearAISessionActiveArtifact(): UseMutationResult<
  void,
  Error,
  string
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (sessionID) => ClearAISessionActiveArtifact(sessionID),
    onSuccess: (_data, sessionID) => {
      qc.invalidateQueries({ queryKey: KEY.sessions });
      qc.invalidateQueries({ queryKey: KEY.session(sessionID) });
    },
  });
}

// useStartAIAssistant 发起一次 AI 对话或工作流生成；结果通过 ai:assistant 事件流式返回。
export function useStartAIAssistant(): UseMutationResult<
  void,
  Error,
  AIAssistantRequest
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (request) => StartAIAssistant(request as never),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: ["workflows"] });
      qc.invalidateQueries({ queryKey: ["assembles"] });
      qc.invalidateQueries({ queryKey: KEY.sessions });
      qc.invalidateQueries({ queryKey: KEY.session(vars.session_id) });
    },
  });
}

// useApplyAIPendingDraft 应用会话中的待确认修改草案；成功后刷新工作流与会话。
export function useApplyAIPendingDraft(): UseMutationResult<
  WorkflowDef,
  Error,
  string
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (sessionID) =>
      ApplyAIPendingDraft(sessionID) as Promise<WorkflowDef>,
    onSuccess: (wf, sessionID) => {
      qc.setQueryData(["workflows", wf.id], wf);
      qc.invalidateQueries({ queryKey: ["workflows"] });
      qc.invalidateQueries({ queryKey: KEY.session(sessionID) });
    },
  });
}

// useDiscardAIPendingDraft 放弃会话中的待确认修改草案（幂等）。
export function useDiscardAIPendingDraft(): UseMutationResult<
  void,
  Error,
  string
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (sessionID) => DiscardAIPendingDraft(sessionID),
    onSuccess: (_data, sessionID) => {
      qc.invalidateQueries({ queryKey: KEY.session(sessionID) });
    },
  });
}
