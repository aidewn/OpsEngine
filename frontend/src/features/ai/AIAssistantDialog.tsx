// AI 助手对话框：左侧多会话列表，右侧消息流。
// 会话持久化在后端，刷新/重启后仍能继续。
// 创建新会话时只需选择环境；SSH 配置是可选范围，环境级会话可用于整体分析。

import {
  FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useNavigate } from 'react-router-dom';
import { EventsOn } from '@wails/runtime/runtime';
import { RotateCcw } from 'lucide-react';
import { Dialog } from '@/components/ui/Dialog';
import { Button } from '@/components/ui/Button';
import { ActionCard } from '@/components/ui/ActionCard';
import { Label } from '@/components/ui/Label';
import { ProgressTimeline } from '@/components/ui/ProgressTimeline';
import { Textarea } from '@/components/ui/Textarea';
import { useRunWorkflow } from '@/api/executions';
import {
  useAISession,
  useAISessions,
  useCreateAISession,
  useDeleteAISession,
  useStartAIAssistant,
  useUpdateAISessionTitle,
} from '@/api/ai';
import { useEnvironments } from '@/api/environments';
import { useSaveAssistantMessageAsDoc } from '@/api/opsDocs';
import { cn } from '@/lib/cn';
import { toast } from '@/lib/toast';
import { hasWailsRuntime } from '@/lib/wailsRuntime';
import { useTabs } from '@/features/tabs/TabsContext';
import type {
  AIAssistantEvent,
  AITargetOption,
  AISession,
  AISessionMessage,
} from '@/types/ai';

interface AIAssistantDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialMessage?: string;
  focusInputOnOpen?: boolean;
}

interface AIAssistantPanelProps {
  active?: boolean;
  selectedSessionID?: string | null;
  onSelectedSessionChange?: (id: string | null) => void;
  initialMessage?: string;
  focusInputOnOpen?: boolean;
  showSessionSidebar?: boolean;
  className?: string;
  onNavigateAway?: () => void;
}

// PendingTurn 是当前正在进行中的一轮对话的临时本地状态，done/error 后归零。
interface PendingTurn {
  sessionID: string;
  requestID: string;
  userContent: string;
  assistantContent: string;
  assistantProgress: string[];
  workflowID?: string;
  workflowName?: string;
  assembleID?: string;
  assembleName?: string;
  artifactType?: 'assemble' | 'workflow';
  actionType?: 'create' | 'update';
  docID?: string;
  docTitle?: string;
  targetOptions?: AITargetOption[];
  targetText?: string;
  selectingTargetID?: string;
  errorText?: string;
}

interface EditingArtifact {
  type: 'assemble' | 'workflow';
  id: string;
  name: string;
}

export function AIAssistantDialog({
  open,
  onOpenChange,
  initialMessage,
  focusInputOnOpen = false,
}: AIAssistantDialogProps) {
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="AI 助手"
      description="多会话保存在本地；只选环境时为环境级会话，Agent 可看到环境内全部配置；选定 SSH 配置时会绑定到该机器并自动采集服务器信息。"
      contentClassName="max-w-5xl"
    >
      <AIAssistantPanel
        active={open}
        initialMessage={initialMessage}
        focusInputOnOpen={focusInputOnOpen}
        showSessionSidebar
        className="h-[560px]"
        onNavigateAway={() => onOpenChange(false)}
      />
    </Dialog>
  );
}

// AIAssistantPanel 是 Chat tab 和 Dialog 共用的主对话面板。
export function AIAssistantPanel({
  active = true,
  selectedSessionID,
  onSelectedSessionChange,
  initialMessage,
  focusInputOnOpen = false,
  showSessionSidebar = true,
  className,
  onNavigateAway,
}: AIAssistantPanelProps) {
  const navigate = useNavigate();
  const { data: sessions } = useAISessions();
  const { data: environments } = useEnvironments();
  const createSession = useCreateAISession();
  const deleteSession = useDeleteAISession();
  const renameSession = useUpdateAISessionTitle();
  const startAssistant = useStartAIAssistant();

  const [internalSelectedID, setInternalSelectedID] = useState<string | null>(null);
  const selectedID = selectedSessionID !== undefined ? selectedSessionID : internalSelectedID;
  const { data: session } = useAISession(selectedID);

  // 新会话表单状态。
  const [draftEnvID, setDraftEnvID] = useState('');
  const [draftConfigID, setDraftConfigID] = useState('');

  const [input, setInput] = useState('');
  const [pending, setPending] = useState<PendingTurn | null>(null);
  const [editingArtifact, setEditingArtifact] = useState<EditingArtifact | null>(null);
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const pendingRef = useRef<PendingTurn | null>(null);
  pendingRef.current = pending;

  const busy = pending !== null && !pending.errorText;

  const sshConfigs = useMemo(() => {
    const env = environments?.find((item) => item.id === draftEnvID);
    return env?.configs.filter((config) => config.kind === 'ssh') ?? [];
  }, [draftEnvID, environments]);

  const setSelectedID = useCallback(
    (id: string | null) => {
      if (selectedSessionID === undefined) {
        setInternalSelectedID(id);
      }
      onSelectedSessionChange?.(id);
    },
    [onSelectedSessionChange, selectedSessionID],
  );

  // 面板失活时清掉本地 in-flight 状态，避免下次打开看到陈旧 streaming 残影。
  useEffect(() => {
    if (!active) {
      setPending(null);
      setInput('');
    } else if (initialMessage) {
      setInput(initialMessage);
    }
  }, [active, initialMessage]);

  // 快捷键打开 AI 助手后，把焦点放到输入区，方便连续键盘操作。
  useEffect(() => {
    if (!active || !focusInputOnOpen) return undefined;
    const timer = window.setTimeout(() => inputRef.current?.focus(), 0);
    return () => window.clearTimeout(timer);
  }, [active, focusInputOnOpen]);

  // 订阅 Wails 后端 ai:assistant 事件。
  useEffect(() => {
    if (!hasWailsRuntime()) return undefined;

    const off = EventsOn('ai:assistant', (event: AIAssistantEvent) => {
      const current = pendingRef.current;
      if (!current || current.requestID !== event.request_id) return;

      if (event.type === 'delta') {
        setPending((prev) =>
          prev
            ? { ...prev, assistantContent: prev.assistantContent + (event.text ?? '') }
            : prev,
        );
      } else if (event.type === 'progress') {
        setPending((prev) =>
          prev
            ? {
                ...prev,
                assistantProgress: [...prev.assistantProgress, event.text ?? ''],
              }
            : prev,
        );
      } else if (event.type === 'workflow') {
        setPending((prev) =>
          prev
            ? {
                ...prev,
                workflowID: event.workflow_id,
                workflowName: event.workflow_name,
                artifactType: event.artifact_type === 'workflow' ? 'workflow' : 'workflow',
                actionType: event.action_type,
              }
            : prev,
        );
      } else if (event.type === 'assemble') {
        setPending((prev) =>
          prev
            ? {
                ...prev,
                assembleID: event.assemble_id,
                assembleName: event.assemble_name,
                artifactType: event.artifact_type === 'assemble' ? 'assemble' : 'assemble',
                actionType: event.action_type,
              }
            : prev,
        );
      } else if (event.type === 'doc') {
        setPending((prev) =>
          prev
            ? {
                ...prev,
                docID: event.doc_id,
                docTitle: event.doc_title,
              }
            : prev,
        );
      } else if (event.type === 'target_select') {
        setPending((prev) =>
          prev
            ? {
                ...prev,
                assistantContent: event.text ?? '请选择要使用的 SSH 配置。',
                targetText: event.text,
                targetOptions: event.target_options ?? [],
              }
            : prev,
        );
      } else if (event.type === 'done') {
        // 服务端已经把这一轮持久化到 session，清掉本地 pending。
        setPending(null);
      } else if (event.type === 'error') {
        setPending((prev) =>
          prev ? { ...prev, errorText: event.text ?? 'AI 调用失败' } : prev,
        );
      }
    });
    return off;
  }, []);

  const handleSelectSession = useCallback(
    (id: string) => {
      if (busy) return;
      setSelectedID(id);
      setPending(null);
      setInput('');
    },
    [busy],
  );

  const handleNewSession = useCallback(() => {
    if (busy) return;
    setSelectedID(null);
    setDraftEnvID('');
    setDraftConfigID('');
    setPending(null);
    setInput('');
  }, [busy]);

  async function handleDeleteSession(id: string) {
    if (busy) return;
    if (!confirm('确认删除该会话？')) return;
    try {
      await deleteSession.mutateAsync(id);
      if (selectedID === id) {
        setSelectedID(null);
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '删除失败');
    }
  }

  async function handleRenameSession(id: string, currentTitle: string) {
    const next = prompt('会话名称', currentTitle);
    if (next === null) return;
    const trimmed = next.trim();
    if (!trimmed || trimmed === currentTitle) return;
    try {
      await renameSession.mutateAsync({ id, title: trimmed });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '重命名失败');
    }
  }

  // handleResend 把一条历史用户消息填回输入框并聚焦，让用户检查后按 Enter 重发。
  // 不直接自动提交：避免误点导致重复请求；同时方便用户在重发前微调措辞。
  function handleResend(content: string) {
    if (busy) return;
    setInput(content);
    window.setTimeout(() => inputRef.current?.focus(), 0);
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    const message = input.trim();
    if (!message || busy) return;

    let sessionID = selectedID;
    if (!sessionID) {
      try {
        const created = await createSession.mutateAsync({
          environment_id: draftEnvID,
          config_id: draftConfigID,
        });
        sessionID = created.id;
        setSelectedID(sessionID);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : '创建会话失败');
        return;
      }
    }

    const requestID = newID();
    setPending({
      sessionID,
      requestID,
      userContent: message,
      assistantContent: '',
      assistantProgress: [],
    });
    setInput('');

    try {
      await startAssistant.mutateAsync({
      request_id: requestID,
      session_id: sessionID,
      operation: editingArtifact
        ? editingArtifact.type === 'assemble'
          ? 'update_assemble'
          : 'update_workflow'
        : 'auto',
      message,
      artifact_type: editingArtifact?.type,
      artifact_id: editingArtifact?.id,
    });
    } catch (err) {
      setPending((prev) =>
        prev
          ? {
              ...prev,
              errorText: err instanceof Error ? err.message : 'AI 调用失败',
            }
          : prev,
      );
    }
  }

  function handleOpenWorkflow(workflowID: string) {
    onNavigateAway?.();
    navigate(`/workflows/${workflowID}`);
  }

  function handleOpenAssemble(assembleID: string) {
    onNavigateAway?.();
    navigate(`/assembles/${assembleID}`);
  }

  async function handleSelectTarget(configID: string) {
    const current = pendingRef.current;
    if (!current || !current.sessionID || current.selectingTargetID) return;
    setPending((prev) =>
      prev
        ? {
            ...prev,
            selectingTargetID: configID,
            assistantProgress: [...prev.assistantProgress, '已选择目标 SSH，继续生成巡检工作流'],
          }
        : prev,
    );
    try {
      await startAssistant.mutateAsync({
        request_id: current.requestID,
        session_id: current.sessionID,
        operation: 'auto',
        message: current.userContent,
        target_config_id: configID,
      });
    } catch (err) {
      setPending((prev) =>
        prev
          ? {
              ...prev,
              selectingTargetID: undefined,
              errorText: err instanceof Error ? err.message : 'AI 调用失败',
            }
          : prev,
      );
    }
  }

  return (
    <div className={cn('flex min-h-0 gap-4', className)}>
      {showSessionSidebar ? (
        <SessionSidebar
            sessions={sessions ?? []}
            selectedID={selectedID}
            busy={busy}
            onSelect={handleSelectSession}
            onNew={handleNewSession}
            onDelete={handleDeleteSession}
            onRename={handleRenameSession}
          />
      ) : null}

      <div className="flex min-w-0 flex-1 flex-col rounded-md border border-ops-border-subtle bg-ops-surface">
        {selectedID && session ? (
          <SessionHeader session={session} environments={environments ?? []} />
        ) : (
          <NewSessionHeader
            environments={environments ?? []}
            envID={draftEnvID}
            configID={draftConfigID}
            sshConfigs={sshConfigs}
            onEnvChange={(value) => {
              setDraftEnvID(value);
              setDraftConfigID('');
            }}
            onConfigChange={setDraftConfigID}
          />
        )}

        <div className="flex-1 overflow-auto bg-ops-canvas px-4 py-3">
          <MessageList
            session={session}
            currentSessionID={selectedID}
            pending={pending}
            onOpenWorkflow={handleOpenWorkflow}
            onOpenAssemble={handleOpenAssemble}
            onContinueArtifact={setEditingArtifact}
            onSelectTarget={handleSelectTarget}
            onResend={handleResend}
          />
        </div>

        <form onSubmit={handleSubmit} className="border-t border-ops-border-subtle p-3">
          {editingArtifact ? (
            <div className="mb-2 flex items-center justify-between rounded-md border border-ops-border-subtle bg-ops-input px-3 py-2 text-xs text-ops-secondary">
              <span>
                正在修改：<span className="text-ops-primary">{editingArtifact.name}</span>
              </span>
              <button
                type="button"
                className="text-ops-tertiary hover:text-ops-primary"
                onClick={() => setEditingArtifact(null)}
              >
                取消
              </button>
            </div>
          ) : null}
          <Textarea
            ref={inputRef}
            value={input}
            onChange={(event) => setInput(event.target.value)}
            rows={3}
            placeholder="例：分析一下这个环境的状态，或：生成一个服务器巡检工作流。"
            disabled={busy}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
                event.preventDefault();
                (event.currentTarget.form as HTMLFormElement | null)?.requestSubmit();
              }
            }}
          />
          <div className="mt-2 flex items-center justify-between">
            <span className="text-xs text-ops-tertiary">Ctrl/Cmd + Enter 发送</span>
            <Button type="submit" disabled={busy}>
              {busy ? '处理中…' : '发送'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}

// SessionSidebar 渲染左侧会话列表与新建按钮。
function SessionSidebar({
  sessions,
  selectedID,
  busy,
  onSelect,
  onNew,
  onDelete,
  onRename,
}: {
  sessions: AISession[];
  selectedID: string | null;
  busy: boolean;
  onSelect: (id: string) => void;
  onNew: () => void;
  onDelete: (id: string) => void;
  onRename: (id: string, currentTitle: string) => void;
}) {
  return (
    <aside className="flex w-56 shrink-0 flex-col rounded-md border border-ops-border-subtle bg-ops-surface">
      <div className="border-b border-ops-border-subtle p-2">
        <Button
          type="button"
          variant="secondary"
          size="sm"
          className="w-full"
          onClick={onNew}
          disabled={busy}
        >
          + 新对话
        </Button>
      </div>
      <ul className="flex-1 divide-y divide-ops-border-subtle overflow-auto">
        {sessions.length === 0 && (
          <li className="px-3 py-4 text-xs text-ops-tertiary">还没有会话</li>
        )}
        {sessions.map((s) => (
          <li
            key={s.id}
            className={cn(
              'group flex cursor-pointer items-center justify-between gap-1 px-3 py-2 hover:bg-ops-elevated',
              selectedID === s.id && 'bg-ops-elevated',
            )}
            onClick={() => onSelect(s.id)}
          >
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm font-medium text-ops-primary">
                {s.title}
              </div>
              <div className="mt-0.5 truncate text-[11px] text-ops-tertiary">
                {new Date(s.updated_at).toLocaleString()}
              </div>
            </div>
            <div className="hidden gap-1 group-hover:flex">
              <button
                type="button"
                className="rounded p-1 text-ops-secondary hover:bg-ops-surface hover:text-ops-primary"
                title="重命名"
                disabled={busy}
                onClick={(event) => {
                  event.stopPropagation();
                  onRename(s.id, s.title);
                }}
              >
                ✎
              </button>
              <button
                type="button"
                className="rounded p-1 text-ops-secondary hover:bg-ops-danger-soft hover:text-ops-danger"
                title="删除"
                disabled={busy}
                onClick={(event) => {
                  event.stopPropagation();
                  onDelete(s.id);
                }}
              >
                🗑
              </button>
            </div>
          </li>
        ))}
      </ul>
    </aside>
  );
}

// NewSessionHeader 渲染未选会话时的环境选择面板。
function NewSessionHeader({
  environments,
  envID,
  configID,
  sshConfigs,
  onEnvChange,
  onConfigChange,
}: {
  environments: { id: string; name: string }[];
  envID: string;
  configID: string;
  sshConfigs: { id: string; name: string }[];
  onEnvChange: (value: string) => void;
  onConfigChange: (value: string) => void;
}) {
  return (
    <div className="grid gap-3 border-b border-ops-border-subtle p-3 sm:grid-cols-2">
      <div className="space-y-1">
        <Label htmlFor="ai-environment">上下文环境（可选）</Label>
        <select
          id="ai-environment"
          value={envID}
          onChange={(event) => onEnvChange(event.target.value)}
          className="h-9 w-full rounded-md border border-ops-border-strong bg-ops-input px-3 text-sm text-ops-primary outline-none focus:border-ops-border-focus"
        >
          <option value="">不指定环境（通用资产生成）</option>
          {environments.map((env) => (
            <option key={env.id} value={env.id}>
              {env.name}
            </option>
          ))}
        </select>
      </div>
      <div className="space-y-1">
        <Label htmlFor="ai-ssh-config">SSH 配置（可选）</Label>
        <select
          id="ai-ssh-config"
          value={configID}
          onChange={(event) => onConfigChange(event.target.value)}
          disabled={!envID}
          className="h-9 w-full rounded-md border border-ops-border-strong bg-ops-input px-3 text-sm text-ops-primary outline-none focus:border-ops-border-focus disabled:text-ops-tertiary"
        >
          <option value="">不指定（环境级会话）</option>
          {sshConfigs.map((config) => (
            <option key={config.id} value={config.id}>
              {config.name}
            </option>
          ))}
        </select>
      </div>
    </div>
  );
}

// SessionHeader 渲染当前会话的环境/配置只读信息。
function SessionHeader({
  session,
  environments,
}: {
  session: AISession;
  environments: { id: string; name: string; configs: { id: string; name: string }[] }[];
}) {
  const env = environments.find((e) => e.id === session.environment_id);
  const config = session.config_id
    ? env?.configs.find((c) => c.id === session.config_id)
    : undefined;
  const scopeLabel = session.scope === 'general' ? '通用会话' : session.scope === 'environment' || !session.config_id ? '环境级' : 'SSH';
  return (
    <div className="flex items-center justify-between border-b border-ops-border-subtle px-3 py-2 text-xs text-ops-secondary">
      <div className="truncate">
        <span className="text-ops-primary">{session.title}</span>
        <span className="mx-2 text-ops-tertiary">·</span>
        {session.scope === 'general' ? (
          <>{scopeLabel} · 可生成/修改集合与工作流</>
        ) : (
          <>
            环境 {env?.name ?? session.environment_id}
            <span className="mx-1 text-ops-tertiary">/</span>
            {scopeLabel} {config?.name ?? session.config_id ?? '全部配置'}
          </>
        )}
      </div>
      {session.context_prefetched && (
        <span className="rounded-sm bg-ops-success-soft px-1.5 py-0.5 text-[10px] text-ops-success">
          已采集服务器信息
        </span>
      )}
    </div>
  );
}

// MessageList 把会话历史与当前 pending 轮次合并渲染。
function MessageList({
  session,
  currentSessionID,
  pending,
  onOpenWorkflow,
  onOpenAssemble,
  onContinueArtifact,
  onSelectTarget,
  onResend,
}: {
  session: AISession | undefined;
  currentSessionID: string | null;
  pending: PendingTurn | null;
  onOpenWorkflow: (workflowID: string) => void;
  onOpenAssemble: (assembleID: string) => void;
  onContinueArtifact: (artifact: EditingArtifact) => void;
  onSelectTarget: (configID: string) => void;
  // onResend 在用户消息 hover 时显示"重发"按钮；点击会把消息文本填回输入框并聚焦。
  onResend?: (content: string) => void;
}) {
  const visible = useMemo(() => {
    if (!session) return [];
    return session.messages.filter(
      (m) => !m.hidden && (m.role === 'user' || m.role === 'assistant'),
    );
  }, [session]);

  const showPending = pending && pending.sessionID === (session?.id ?? currentSessionID ?? '');

  // Run() 一进入就把用户消息 append + save 到 session，invalidate 后 visible 立刻包含它。
  // 此时如果继续渲染 pending 的 user 气泡，用户消息会显示两次。
  // 判定：visible 中最后一条 user 消息内容若与 pending.userContent 一致，说明已经持久化，pending user 不再渲染。
  const pendingUserAlreadyPersisted = useMemo(() => {
    if (!pending) return false;
    for (let i = visible.length - 1; i >= 0; i--) {
      const m = visible[i]!;
      if (m.role === 'user') {
        return m.content === pending.userContent;
      }
    }
    return false;
  }, [pending, visible]);

  if (visible.length === 0 && !showPending) {
    return (
      <div className="text-sm text-ops-secondary">
        选择左侧会话继续对话，或输入新需求。例如「分析一下这个环境目前的状态」。
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {visible.map((message) => (
        <Bubble
          key={message.id}
          message={message}
          sessionID={session?.id}
          onOpenWorkflow={onOpenWorkflow}
          onOpenAssemble={onOpenAssemble}
          onContinueArtifact={onContinueArtifact}
          onResend={onResend}
        />
      ))}
      {showPending && pending && (
        <>
          {!pendingUserAlreadyPersisted ? (
            <Bubble
              message={{
                id: pending.requestID + ':user',
                role: 'user',
                content: pending.userContent,
                created_at: new Date().toISOString(),
              }}
              onOpenWorkflow={onOpenWorkflow}
              onOpenAssemble={onOpenAssemble}
              onContinueArtifact={onContinueArtifact}
            />
          ) : null}
          <Bubble
            message={{
              id: pending.requestID + ':assistant',
              role: 'assistant',
              content: pending.errorText
                ? pending.errorText
                : pending.assistantContent,
              progress: pending.assistantProgress,
              workflow_id: pending.workflowID,
              workflow_name: pending.workflowName,
              assemble_id: pending.assembleID,
              assemble_name: pending.assembleName,
              artifact_type: pending.artifactType,
              action_type: pending.actionType,
              doc_id: pending.docID,
              doc_title: pending.docTitle,
              target_options: pending.targetOptions,
              created_at: new Date().toISOString(),
            }}
            onOpenWorkflow={onOpenWorkflow}
            onOpenAssemble={onOpenAssemble}
            onContinueArtifact={onContinueArtifact}
            onSelectTarget={onSelectTarget}
            selectingTargetID={pending.selectingTargetID}
          />
        </>
      )}
    </div>
  );
}

// Bubble 渲染单条会话消息气泡。
function Bubble({
  message,
  sessionID,
  onOpenWorkflow,
  onOpenAssemble,
  onContinueArtifact,
  onSelectTarget,
  selectingTargetID,
  onResend,
}: {
  message: AISessionMessage & { target_options?: AITargetOption[] };
  sessionID?: string;
  onOpenWorkflow: (workflowID: string) => void;
  onOpenAssemble: (assembleID: string) => void;
  onContinueArtifact: (artifact: EditingArtifact) => void;
  onSelectTarget?: (configID: string) => void;
  selectingTargetID?: string;
  onResend?: (content: string) => void;
}) {
  const isUser = message.role === 'user';
  // 重发按钮：用户气泡 hover 时显示；用 group/group-hover 类做隐藏-显现切换。
  // 仅对真实持久化的用户消息开放（pending bubble 的 id 形如 "<req>:user" 与持久化消息无歧义）。
  const canResend = isUser && !!onResend && message.content.length > 0;
  const navigate = useNavigate();
  const runWorkflow = useRunWorkflow();
  const { openTab } = useTabs();
  // 触发"保存为报告"的条件：
  //   - 必须是已落库的 assistant 消息（pending 消息没 sessionID/messageID，不允许保存）
  //   - 不能是工作流类（已有"打开工作流"入口）
  //   - 必须带 intent 标签（chat / troubleshoot），避免把 system 提示等误存
  const canSaveAsDoc =
    !isUser &&
    !!sessionID &&
    !!message.id &&
    !message.workflow_id &&
    !message.assemble_id &&
    (message.intent === 'troubleshoot' || message.intent === 'chat');
  return (
    <div className={cn('group flex', isUser ? 'justify-end' : 'justify-start')}>
      {/* 用户消息：hover 时左侧显示"重发"按钮。放在气泡外侧避免遮挡内容。 */}
      {canResend ? (
        <button
          type="button"
          className="mr-2 mt-1 self-start rounded-md p-1 text-ops-tertiary opacity-0 transition-opacity hover:bg-ops-elevated hover:text-ops-primary group-hover:opacity-100"
          title="重发这条消息（填回输入框）"
          aria-label="重发"
          onClick={() => onResend?.(message.content)}
        >
          <RotateCcw size={14} />
        </button>
      ) : null}
      <div
        className={cn(
          'max-w-[82%] rounded-md px-3 py-2 text-sm leading-6',
          isUser
            ? 'bg-ops-accent text-ops-inverse'
            : 'border border-ops-border-subtle bg-ops-surface text-ops-primary',
        )}
      >
        {message.content && (
          <div className="whitespace-pre-wrap">{message.content}</div>
        )}
        {!message.content && !isUser && (
          <div className="text-ops-tertiary">正在处理…</div>
        )}
        {message.progress && message.progress.length > 0 && (
          <ProgressTimeline items={message.progress} />
        )}
        {message.workflow_id && (
          <ActionCard
            title={message.workflow_name || 'AI 生成工作流'}
            description="可直接打开画布，也可以立即启动一次执行。"
            tone="success"
            secondaryAction={{
              label: '打开工作流',
              onClick: () => onOpenWorkflow(message.workflow_id!),
            }}
            tertiaryAction={{
              label: '继续修改',
              onClick: () =>
                onContinueArtifact({
                  type: 'workflow',
                  id: message.workflow_id!,
                  name: message.workflow_name || 'AI 生成工作流',
                }),
            }}
            primaryAction={{
              label: '运行此工作流',
              loading: runWorkflow.isPending,
              onClick: async () => {
                const workflowID = message.workflow_id!;
                try {
                  const execID = await runWorkflow.mutateAsync(workflowID);
                  openTab({
                    kind: 'execution',
                    id: execID,
                    name: `${message.workflow_name || '工作流'} #${execID.slice(0, 4)}`,
                  });
                  toast.info('已开始执行', {
                    label: '查看进度',
                    onClick: () => navigate(`/executions/${execID}`),
                  });
                } catch (err) {
                  toast.error(err instanceof Error ? err.message : '启动执行失败');
                }
              },
            }}
          />
        )}
        {message.assemble_id && (
          <ActionCard
            title={message.assemble_name || 'AI 生成集合'}
            description="可复用集合已保存，可继续迭代或打开画布查看。"
            tone="success"
            secondaryAction={{
              label: '打开集合',
              onClick: () => onOpenAssemble(message.assemble_id!),
            }}
            primaryAction={{
              label: '继续修改',
              onClick: () =>
                onContinueArtifact({
                  type: 'assemble',
                  id: message.assemble_id!,
                  name: message.assemble_name || 'AI 生成集合',
                }),
            }}
          />
        )}
        {message.doc_id && (
          <ActionCard
            title={message.doc_title || 'AI 生成文档'}
            description={message.doc_id}
            tone="info"
            primaryAction={{
              label: '查看文档',
              onClick: () => navigate(`/?tab=reports&doc=${message.doc_id}`),
            }}
          />
        )}
        {!isUser && message.target_options && message.target_options.length > 0 && (
          <div className="mt-3 space-y-3 rounded-md border border-ops-warning bg-ops-warning-soft px-3 py-3 shadow-[inset_3px_0_0_#F59E0B]">
            <div className="text-sm font-medium text-ops-warning">Agent 等待你的选择</div>
            <div className="text-xs text-ops-primary">选择巡检目标 SSH 后，Agent 会继续生成工作流。</div>
            <div className="flex flex-wrap gap-2">
              {message.target_options.map((option) => (
                <Button
                  key={option.id}
                  type="button"
                  size="sm"
                  variant="secondary"
                  disabled={!onSelectTarget || !!selectingTargetID}
                  onClick={() => onSelectTarget?.(option.id)}
                >
                  {selectingTargetID === option.id ? '继续中…' : option.name || option.id}
                </Button>
              ))}
            </div>
          </div>
        )}
        {canSaveAsDoc && (
          <SaveAsDocButton sessionID={sessionID!} messageID={message.id} intent={message.intent} />
        )}
      </div>
    </div>
  );
}

// SaveAsDocButton 把当前 assistant 消息保存为 OpsDoc。
// 成功后展示"已保存"短反馈 + 文档 ID（截断）；失败时弹 alert。
// 不直接跳转文档库，避免打断用户当前对话；用户可自行切到文档 tab 查看。
function SaveAsDocButton({
  sessionID,
  messageID,
  intent,
}: {
  sessionID: string;
  messageID: string;
  intent?: string;
}) {
  const save = useSaveAssistantMessageAsDoc();
  const [saved, setSaved] = useState<{ id: string } | null>(null);
  async function handleClick() {
    try {
      const doc = await save.mutateAsync({ sessionID, messageID });
      setSaved({ id: doc.id });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    }
  }
  const label = intent === 'troubleshoot' ? '保存为排障报告' : '保存为报告';
  return (
    <div className="mt-3 flex items-center justify-end gap-2">
      {saved && (
        <span className="text-[11px] text-emerald-600">
          已保存（ID: <span className="font-mono">{saved.id.slice(0, 8)}</span>）
        </span>
      )}
      <Button
        type="button"
        size="sm"
        variant="ghost"
        onClick={handleClick}
        disabled={save.isPending || !!saved}
      >
        {save.isPending ? '保存中…' : saved ? '已保存' : `📄 ${label}`}
      </Button>
    </div>
  );
}

// newID 生成前端请求和消息 ID。
function newID() {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}
