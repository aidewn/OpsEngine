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
import { RotateCcw, ChevronDown, Sparkles } from 'lucide-react';
import { Dialog } from '@/components/ui/Dialog';
import { Button } from '@/components/ui/Button';
import { ActionCard } from '@/components/ui/ActionCard';
import { EmptyState } from '@/components/ui/EmptyState';
import { Label } from '@/components/ui/Label';
import { MarkdownView } from '@/components/ui/MarkdownView';
import { ProgressTimeline } from '@/components/ui/ProgressTimeline';
import { Textarea } from '@/components/ui/Textarea';
import { useQueryClient } from '@tanstack/react-query';
import { useRunWorkflow } from '@/api/executions';
import {
  useAISession,
  useAISessions,
  useClearAISessionActiveArtifact,
  useCreateAISession,
  useDeleteAISession,
  useSetAISessionActiveArtifact,
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
  /** 嵌入首页主区域时使用，去掉 Dialog 风格边框并撑满高度 */
  embedded?: boolean;
  className?: string;
  onNavigateAway?: () => void;
}

// 空会话时的快捷提问，点击后填入输入框
const QUICK_PROMPTS = [
  '分析一下这个环境目前的运行状态',
  '帮我生成一个服务器巡检工作流',
  '排查 Docker 容器启动失败的问题',
];

// PendingTurn 是当前正在进行中的一轮对话的临时本地状态，done/error 后归零。
interface PendingTurn {
  sessionID: string;
  requestID: string;
  userContent: string;
  assistantContent: string;
  assistantProgress: string[];
  // 瞬态心跳文本，每次替换不追加；done/delta 到来时清空
  assistantHeartbeat?: string;
  workflowID?: string;
  workflowName?: string;
  assembleID?: string;
  assembleName?: string;
  artifactType?: 'assemble' | 'workflow';
  actionType?: 'create' | 'update';
  docID?: string;
  docTitle?: string;
  nodeCount?: number;
  changeSummary?: string;
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
  embedded = false,
  className,
  onNavigateAway,
}: AIAssistantPanelProps) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { data: sessions } = useAISessions();
  const { data: environments } = useEnvironments();
  const createSession = useCreateAISession();
  const deleteSession = useDeleteAISession();
  const renameSession = useUpdateAISessionTitle();
  const startAssistant = useStartAIAssistant();
  const setActiveArtifact = useSetAISessionActiveArtifact();
  const clearActiveArtifact = useClearAISessionActiveArtifact();

  const [internalSelectedID, setInternalSelectedID] = useState<string | null>(null);
  const selectedID = selectedSessionID !== undefined ? selectedSessionID : internalSelectedID;
  const { data: session } = useAISession(selectedID);

  // 新会话表单状态。
  const [draftEnvID, setDraftEnvID] = useState('');
  const [draftConfigID, setDraftConfigID] = useState('');

  const [input, setInput] = useState('');
  const [pending, setPending] = useState<PendingTurn | null>(null);
  const [stickToBottom, setStickToBottom] = useState(true);
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const scrollContainerRef = useRef<HTMLDivElement | null>(null);
  const messagesEndRef = useRef<HTMLDivElement | null>(null);
  const pendingRef = useRef<PendingTurn | null>(null);
  pendingRef.current = pending;

  // finalizeAssistantTurn 在 RPC 返回后兜底结束本轮 UI（Wails 同步调用时 done 事件可能晚于 mutate  resolve）。
  const finalizeAssistantTurn = useCallback(
    async (sessionID: string) => {
      await new Promise<void>((resolve) => {
        window.setTimeout(resolve, 0);
      });
      setPending((prev) => {
        if (!prev) return prev;
        if (prev.errorText) return prev;
        if (prev.targetOptions?.length) return prev;
        return null;
      });
      void queryClient.invalidateQueries({ queryKey: ['ai', 'session', sessionID] });
    },
    [queryClient],
  );

  const busy = pending !== null && !pending.errorText;
  const waitingForTarget =
    !!pending?.targetOptions?.length && !pending.errorText && !pending.selectingTargetID;

  // 从会话持久化的 active_artifact 推导编辑模式（Claude Code 式 artifact 上下文）。
  const editingArtifact = useMemo<EditingArtifact | null>(() => {
    if (!session?.active_artifact_id || !session.active_artifact_type) return null;
    if (session.active_artifact_type !== 'workflow' && session.active_artifact_type !== 'assemble') {
      return null;
    }
    return {
      type: session.active_artifact_type,
      id: session.active_artifact_id,
      name: session.active_artifact_name || session.active_artifact_id,
    };
  }, [session]);

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

  // 侧栏切换会话时重置输入区；进行中的请求在后台继续，回到该会话后由服务端数据刷新。
  const prevSelectedIDRef = useRef<string | null | undefined>(undefined);
  useEffect(() => {
    if (prevSelectedIDRef.current === undefined) {
      prevSelectedIDRef.current = selectedID;
      return;
    }
    if (prevSelectedIDRef.current === selectedID) return;
    prevSelectedIDRef.current = selectedID;
    // 首条消息创建会话时 selectedID 会变，但 pending 仍属于同一会话，不能清掉
    if (pendingRef.current?.sessionID === selectedID) {
      setInput('');
      return;
    }
    setInput('');
    setPending(null);
    setStickToBottom(true);
  }, [selectedID]);

  // 输入框随内容自动增高，最多约 6 行。
  useEffect(() => {
    const el = inputRef.current;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
  }, [input]);

  // 消息或流式内容变化时，若用户未主动上滑则滚到底部。
  useEffect(() => {
    if (!stickToBottom) return;
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [session?.messages, pending?.assistantContent, pending?.assistantProgress, pending?.assistantHeartbeat, stickToBottom]);

  function handleScrollMessages() {
    const el = scrollContainerRef.current;
    if (!el) return;
    const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
    setStickToBottom(distanceFromBottom < 80);
  }

  function scrollToBottom() {
    setStickToBottom(true);
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }

  function applyQuickPrompt(prompt: string) {
    if (busy) return;
    setInput(prompt);
    window.setTimeout(() => inputRef.current?.focus(), 0);
  }

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
            ? {
                ...prev,
                // 收到真实流式 token，清掉心跳，避免心跳与正文并存
                assistantHeartbeat: undefined,
                assistantContent: prev.assistantContent + (event.text ?? ''),
              }
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
      } else if (event.type === 'heartbeat') {
        // 单行原地刷新，不进 progress 数组，避免持久化时堆积
        setPending((prev) =>
          prev ? { ...prev, assistantHeartbeat: event.text ?? '' } : prev,
        );
      } else if (event.type === 'workflow') {
        setPending((prev) =>
          prev
            ? {
                ...prev,
                workflowID: event.workflow_id,
                workflowName: event.workflow_name,
                artifactType: 'workflow',
                actionType: event.action_type,
                nodeCount: event.node_count,
                changeSummary: event.change_summary,
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
                artifactType: 'assemble',
                actionType: event.action_type,
                nodeCount: event.node_count,
                changeSummary: event.change_summary,
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
        setPending(null);
        if (current.sessionID) {
          void queryClient.invalidateQueries({
            queryKey: ['ai', 'session', current.sessionID],
          });
        }
      } else if (event.type === 'error') {
        const errText = event.text ?? 'AI 调用失败';
        setPending((prev) =>
          prev ? { ...prev, errorText: errText } : prev,
        );
        if (current.sessionID) {
          void queryClient.invalidateQueries({
            queryKey: ['ai', 'session', current.sessionID],
          });
        }
      }
    });
    return off;
  }, [queryClient]);

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

  // sendMessage 发起一轮 AI 请求（创建会话、pending、调用后端）。
  const sendMessage = useCallback(
    async (message: string) => {
      let sessionID = selectedID;
      if (!sessionID) {
        const created = await createSession.mutateAsync({
          environment_id: draftEnvID,
          config_id: draftConfigID,
        });
        sessionID = created.id;
        setSelectedID(sessionID);
      }

      const requestID = newID();
      setPending({
        sessionID,
        requestID,
        userContent: message,
        assistantContent: '',
        assistantProgress: [],
      });

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
        await finalizeAssistantTurn(sessionID);
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
    },
    [
      selectedID,
      createSession,
      draftEnvID,
      draftConfigID,
      editingArtifact,
      startAssistant,
      setSelectedID,
      finalizeAssistantTurn,
    ],
  );

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    const message = input.trim();
    if (!message || busy) return;
    setInput('');
    try {
      await sendMessage(message);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '创建会话失败');
    }
  }

  // handleRetryPending 在失败后一键重试上一条用户消息。
  function handleRetryPending(content: string) {
    if (busy) return;
    setPending(null);
    void sendMessage(content);
  }

  function handleOpenWorkflow(workflowID: string) {
    onNavigateAway?.();
    navigate(`/workflows/${workflowID}`);
  }

  function handleOpenAssemble(assembleID: string) {
    onNavigateAway?.();
    navigate(`/assembles/${assembleID}`);
  }

  async function handleContinueArtifact(artifact: EditingArtifact) {
    if (!selectedID) return;
    try {
      await setActiveArtifact.mutateAsync({
        sessionID: selectedID,
        artifactType: artifact.type,
        artifactID: artifact.id,
        artifactName: artifact.name,
      });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '进入编辑模式失败');
    }
  }

  async function handleExitEditMode() {
    if (!selectedID) return;
    try {
      await clearActiveArtifact.mutateAsync(selectedID);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '退出编辑模式失败');
    }
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
      await finalizeAssistantTurn(current.sessionID);
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

      <div
        className={cn(
          'relative flex min-w-0 flex-1 flex-col',
          embedded
            ? 'h-full min-h-0 bg-ops-canvas'
            : 'rounded-md border border-ops-border-subtle bg-ops-surface',
        )}
      >
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

        <div
          ref={scrollContainerRef}
          onScroll={handleScrollMessages}
          className="relative min-h-0 flex-1 overflow-auto bg-ops-canvas px-4 py-4"
        >
          <MessageList
            session={session}
            currentSessionID={selectedID}
            pending={pending}
            onOpenWorkflow={handleOpenWorkflow}
            onOpenAssemble={handleOpenAssemble}
            onContinueArtifact={handleContinueArtifact}
            onSelectTarget={handleSelectTarget}
            onResend={handleResend}
            onRetryPending={handleRetryPending}
            onQuickPrompt={applyQuickPrompt}
            activeArtifactID={editingArtifact?.id}
          />
          <div ref={messagesEndRef} aria-hidden />
          {!stickToBottom ? (
            <button
              type="button"
              className="sticky bottom-2 left-1/2 z-10 flex -translate-x-1/2 items-center gap-1 rounded-full border border-ops-border-subtle bg-ops-elevated px-3 py-1.5 text-xs text-ops-secondary shadow-lg hover:text-ops-primary"
              onClick={scrollToBottom}
            >
              <ChevronDown size={14} />
              回到底部
            </button>
          ) : null}
        </div>

        <form onSubmit={handleSubmit} className="border-t border-ops-border-subtle bg-ops-surface p-3">
          {waitingForTarget ? (
            <div className="mb-2 flex items-center gap-2 rounded-md border border-ops-warning bg-ops-warning-soft px-3 py-2 text-xs text-ops-warning">
              <span className="inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-ops-warning" />
              Agent 等待你选择巡检目标 SSH，请在上方消息中点击目标后继续。
            </div>
          ) : null}
          {editingArtifact ? (
            <div className="mb-2 flex items-center justify-between rounded-md border border-ops-accent/40 bg-ops-accent-soft px-3 py-2 text-xs text-ops-secondary">
              <span>
                编辑模式 ·{' '}
                <span className="text-ops-primary">
                  {editingArtifact.type === 'workflow' ? '工作流' : '集合'}：{editingArtifact.name}
                </span>
                <span className="ml-2 text-ops-tertiary">后续消息将迭代修改此资产</span>
              </span>
              <button
                type="button"
                className="text-ops-tertiary hover:text-ops-primary"
                onClick={handleExitEditMode}
              >
                退出编辑
              </button>
            </div>
          ) : null}
          <Textarea
            ref={inputRef}
            value={input}
            onChange={(event) => setInput(event.target.value)}
            rows={1}
            placeholder={
              waitingForTarget
                ? '请先在上方选择 SSH 目标…'
                : '输入问题，Enter 发送，Shift+Enter 换行'
            }
            disabled={busy || waitingForTarget}
            className="max-h-40 min-h-[40px] resize-none"
            onKeyDown={(event) => {
              if (event.key === 'Enter' && !event.shiftKey) {
                event.preventDefault();
                if (!event.nativeEvent.isComposing) {
                  (event.currentTarget.form as HTMLFormElement | null)?.requestSubmit();
                }
              }
            }}
          />
          <div className="mt-2 flex items-center justify-between gap-2">
            <span className="text-xs text-ops-tertiary">
              {waitingForTarget ? '等待选择目标' : 'Enter 发送 · Shift+Enter 换行'}
            </span>
            <Button type="submit" disabled={busy || waitingForTarget || !input.trim()}>
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
  onRetryPending,
  onQuickPrompt,
  activeArtifactID,
}: {
  session: AISession | undefined;
  currentSessionID: string | null;
  pending: PendingTurn | null;
  onOpenWorkflow: (workflowID: string) => void;
  onOpenAssemble: (assembleID: string) => void;
  onContinueArtifact: (artifact: EditingArtifact) => void;
  onSelectTarget: (configID: string) => void;
  onResend?: (content: string) => void;
  onRetryPending?: (content: string) => void;
  onQuickPrompt?: (prompt: string) => void;
  activeArtifactID?: string;
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
      <EmptyState
        className="mx-auto mt-8 max-w-lg border-solid bg-ops-surface/40"
        icon={<Sparkles size={28} />}
        title="开始新对话"
        description="选择上方环境（可选），或直接输入需求。Agent 可帮你分析环境、排障、生成工作流与集合。"
        action={
          onQuickPrompt ? (
            <div className="flex w-full max-w-md flex-col gap-2 text-left">
              {QUICK_PROMPTS.map((prompt) => (
                <button
                  key={prompt}
                  type="button"
                  className="rounded-md border border-ops-border-subtle bg-ops-input px-3 py-2 text-left text-sm text-ops-secondary transition-colors hover:border-ops-border-strong hover:text-ops-primary"
                  onClick={() => onQuickPrompt(prompt)}
                >
                  {prompt}
                </button>
              ))}
            </div>
          ) : undefined
        }
      />
    );
  }

  return (
    <div className="space-y-3">
      {visible.map((message) => (
        <Bubble
          key={message.id}
          message={message}
          sessionID={session?.id}
          streaming={false}
          onOpenWorkflow={onOpenWorkflow}
          onOpenAssemble={onOpenAssemble}
          onContinueArtifact={onContinueArtifact}
          onResend={onResend}
          activeArtifactID={activeArtifactID}
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
              streaming={false}
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
              heartbeat: pending.assistantHeartbeat,
              workflow_id: pending.workflowID,
              workflow_name: pending.workflowName,
              assemble_id: pending.assembleID,
              assemble_name: pending.assembleName,
              artifact_type: pending.artifactType,
              action_type: pending.actionType,
              node_count: pending.nodeCount,
              change_summary: pending.changeSummary,
              doc_id: pending.docID,
              doc_title: pending.docTitle,
              target_options: pending.targetOptions,
              isError: !!pending.errorText,
              created_at: new Date().toISOString(),
            }}
            streaming={!pending.errorText && !pending.assistantContent}
            onOpenWorkflow={onOpenWorkflow}
            onOpenAssemble={onOpenAssemble}
            onContinueArtifact={onContinueArtifact}
            onSelectTarget={onSelectTarget}
            selectingTargetID={pending.selectingTargetID}
            onRetry={
              pending.errorText
                ? () => onRetryPending?.(pending.userContent)
                : undefined
            }
            activeArtifactID={undefined}
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
  streaming = false,
  onOpenWorkflow,
  onOpenAssemble,
  onContinueArtifact,
  onSelectTarget,
  selectingTargetID,
  onResend,
  onRetry,
  activeArtifactID,
}: {
  message: AISessionMessage & {
    target_options?: AITargetOption[];
    heartbeat?: string;
    isError?: boolean;
  };
  sessionID?: string;
  /** 流式输出中时用纯文本，避免 Markdown 反复解析闪烁 */
  streaming?: boolean;
  onOpenWorkflow: (workflowID: string) => void;
  onOpenAssemble: (assembleID: string) => void;
  onContinueArtifact: (artifact: EditingArtifact) => void;
  onSelectTarget?: (configID: string) => void;
  selectingTargetID?: string;
  onResend?: (content: string) => void;
  /** 失败时一键重试（填回上一条用户消息并聚焦） */
  onRetry?: () => void;
  activeArtifactID?: string;
}) {
  const isUser = message.role === 'user';
  const isError =
    !!message.isError ||
    (!isUser &&
      (/^\[(配置错误|网络错误|模型响应错误)/.test(message.content) ||
        message.content === 'AI 调用失败'));
  const waitingForTarget =
    !isUser && !!message.target_options && message.target_options.length > 0;
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
            : waitingForTarget
              ? 'border border-ops-warning bg-ops-warning-soft text-ops-primary shadow-[inset_3px_0_0_#F59E0B]'
              : isError
                ? 'border border-ops-danger bg-ops-danger-soft text-ops-danger'
                : 'border border-ops-border-subtle bg-ops-surface text-ops-primary',
        )}
      >
        {message.content ? (
          isUser || streaming || isError ? (
            <div className={cn('whitespace-pre-wrap', isUser && 'text-ops-inverse')}>
              {message.content}
            </div>
          ) : (
            <MarkdownView body={message.content} />
          )
        ) : null}
        {!message.content && !isUser && !(message.progress && message.progress.length > 0) && (
          <div className="flex items-center gap-2 text-ops-tertiary">
            <span className="inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-ops-accent" />
            正在处理…
          </div>
        )}
        {message.progress && message.progress.length > 0 && (
          <ProgressTimeline
            items={message.progress}
            live={streaming}
            defaultOpen={streaming}
          />
        )}
        {message.heartbeat && (
          <div className="mt-1 flex items-center gap-1.5 text-[11px] italic text-ops-tertiary">
            <span className="inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-ops-accent" />
            {message.heartbeat}
          </div>
        )}
        {message.workflow_id && (
          <ActionCard
            title={message.workflow_name || 'AI 生成工作流'}
            badge={artifactActionBadge(message, message.workflow_id, activeArtifactID, streaming)}
            description={artifactCardDescription(
              message.node_count,
              message.change_summary,
              '可直接打开画布、运行一次，或进入编辑模式继续迭代。',
            )}
            tone="success"
            primaryAction={{
              label:
                activeArtifactID === message.workflow_id ? '编辑中' : '继续修改',
              disabled: activeArtifactID === message.workflow_id,
              onClick: () =>
                onContinueArtifact({
                  type: 'workflow',
                  id: message.workflow_id!,
                  name: message.workflow_name || 'AI 生成工作流',
                }),
            }}
            secondaryAction={{
              label: '打开工作流',
              onClick: () => onOpenWorkflow(message.workflow_id!),
            }}
            tertiaryAction={{
              label: runWorkflow.isPending ? '启动中…' : '运行',
              disabled: runWorkflow.isPending,
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
            badge={artifactActionBadge(message, message.assemble_id, activeArtifactID, streaming)}
            description={artifactCardDescription(
              message.node_count,
              message.change_summary,
              '可复用集合已保存，进入编辑模式即可多轮迭代。',
            )}
            tone="success"
            primaryAction={{
              label: activeArtifactID === message.assemble_id ? '编辑中' : '继续修改',
              disabled: activeArtifactID === message.assemble_id,
              onClick: () =>
                onContinueArtifact({
                  type: 'assemble',
                  id: message.assemble_id!,
                  name: message.assemble_name || 'AI 生成集合',
                }),
            }}
            secondaryAction={{
              label: '打开集合',
              onClick: () => onOpenAssemble(message.assemble_id!),
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
          <div className="mt-3 space-y-3">
            <div className="text-sm font-medium text-ops-warning">请选择巡检目标</div>
            <div className="text-xs text-ops-primary">
              选择 SSH 配置后，Agent 会继续生成巡检工作流。
            </div>
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
        {isError && onRetry ? (
          <div className="mt-2">
            <Button type="button" size="sm" variant="secondary" onClick={onRetry}>
              重试
            </Button>
          </div>
        ) : null}
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

// artifactActionBadge 生成工作流/集合 ActionCard 的状态角标。
function artifactActionBadge(
  message: { action_type?: string },
  artifactID: string,
  activeArtifactID?: string,
  inProgress?: boolean,
) {
  if (inProgress) return '处理中';
  if (activeArtifactID === artifactID) return '编辑中';
  if (message.action_type === 'update') return '已更新';
  if (message.action_type === 'create') return '已创建';
  return undefined;
}

// artifactCardDescription 拼接节点数、变更摘要与默认说明。
function artifactCardDescription(
  nodeCount?: number,
  changeSummary?: string,
  fallback?: string,
) {
  const parts: string[] = [];
  if (nodeCount && nodeCount > 0) parts.push(`${nodeCount} 个节点`);
  if (changeSummary) parts.push(changeSummary);
  if (parts.length > 0) return parts.join(' · ');
  return fallback;
}
