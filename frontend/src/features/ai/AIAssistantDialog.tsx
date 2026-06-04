// AI 助手对话框：左侧多会话列表，右侧消息流。
// 会话持久化在后端，刷新/重启后仍能继续。
// 创建新会话时需选 SSH 环境，首条用户消息触发服务器信息预取。

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
import { Dialog } from '@/components/ui/Dialog';
import { Button } from '@/components/ui/Button';
import { Label } from '@/components/ui/Label';
import { Textarea } from '@/components/ui/Textarea';
import {
  useAISession,
  useAISessions,
  useCreateAISession,
  useDeleteAISession,
  useStartAIAssistant,
  useUpdateAISessionTitle,
} from '@/api/ai';
import { useEnvironments } from '@/api/environments';
import { cn } from '@/lib/cn';
import type {
  AIAssistantEvent,
  AISession,
  AISessionMessage,
} from '@/types/ai';

interface AIAssistantDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
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
  errorText?: string;
}

export function AIAssistantDialog({
  open,
  onOpenChange,
}: AIAssistantDialogProps) {
  const navigate = useNavigate();
  const { data: sessions } = useAISessions();
  const { data: environments } = useEnvironments();
  const createSession = useCreateAISession();
  const deleteSession = useDeleteAISession();
  const renameSession = useUpdateAISessionTitle();
  const startAssistant = useStartAIAssistant();

  const [selectedID, setSelectedID] = useState<string | null>(null);
  const { data: session } = useAISession(selectedID);

  // 新会话表单状态。
  const [draftEnvID, setDraftEnvID] = useState('');
  const [draftConfigID, setDraftConfigID] = useState('');

  const [input, setInput] = useState('');
  const [pending, setPending] = useState<PendingTurn | null>(null);
  const pendingRef = useRef<PendingTurn | null>(null);
  pendingRef.current = pending;

  const busy = pending !== null && !pending.errorText;

  const sshConfigs = useMemo(() => {
    const env = environments?.find((item) => item.id === draftEnvID);
    return env?.configs.filter((config) => config.kind === 'ssh') ?? [];
  }, [draftEnvID, environments]);

  // 关闭对话框时不清空选择，重新打开后还能看到上次的会话。
  // 但本地 pending（in-flight 状态）应清掉，避免下次打开看到陈旧 streaming 残影。
  useEffect(() => {
    if (!open) {
      setPending(null);
      setInput('');
    }
  }, [open]);

  // 订阅 Wails 后端 ai:assistant 事件。
  useEffect(() => {
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
      alert(err instanceof Error ? err.message : '删除失败');
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
      alert(err instanceof Error ? err.message : '重命名失败');
    }
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    const message = input.trim();
    if (!message || busy) return;

    let sessionID = selectedID;
    if (!sessionID) {
      if (!draftEnvID || !draftConfigID) {
        setPending({
          sessionID: '',
          requestID: newID(),
          userContent: message,
          assistantContent: '请先选择目标环境与 SSH 配置。',
          assistantProgress: [],
          errorText: '缺少环境',
        });
        return;
      }
      try {
        const created = await createSession.mutateAsync({
          environment_id: draftEnvID,
          config_id: draftConfigID,
        });
        sessionID = created.id;
        setSelectedID(sessionID);
      } catch (err) {
        alert(err instanceof Error ? err.message : '创建会话失败');
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
        operation: 'auto',
        message,
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
    onOpenChange(false);
    navigate(`/workflows/${workflowID}`);
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!busy) onOpenChange(next);
      }}
      title="AI 助手"
      description="多会话保存在本地；首次提问会自动采集所选 SSH 环境的服务器信息作为分析依据。"
      contentClassName="max-w-5xl"
    >
      <div className="flex h-[560px] gap-4">
        <SessionSidebar
          sessions={sessions ?? []}
          selectedID={selectedID}
          busy={busy}
          onSelect={handleSelectSession}
          onNew={handleNewSession}
          onDelete={handleDeleteSession}
          onRename={handleRenameSession}
        />

        <div className="flex flex-1 flex-col rounded-md border border-slate-200">
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

          <div className="flex-1 overflow-auto bg-slate-50 px-4 py-3">
            <MessageList
              session={session}
              pending={pending}
              onOpenWorkflow={handleOpenWorkflow}
            />
          </div>

          <form onSubmit={handleSubmit} className="border-t border-slate-200 p-3">
            <Textarea
              value={input}
              onChange={(event) => setInput(event.target.value)}
              rows={3}
              placeholder="例：分析一下这台服务器的状态，或：生成一个 nginx 配置热加载工作流。"
              disabled={busy}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
                  event.preventDefault();
                  (event.currentTarget.form as HTMLFormElement | null)?.requestSubmit();
                }
              }}
            />
            <div className="mt-2 flex items-center justify-between">
              <span className="text-xs text-slate-400">Ctrl/Cmd + Enter 发送</span>
              <Button type="submit" disabled={busy}>
                {busy ? '处理中…' : '发送'}
              </Button>
            </div>
          </form>
        </div>
      </div>
    </Dialog>
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
    <aside className="flex w-56 shrink-0 flex-col rounded-md border border-slate-200 bg-white">
      <div className="border-b border-slate-200 p-2">
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
      <ul className="flex-1 divide-y divide-slate-100 overflow-auto">
        {sessions.length === 0 && (
          <li className="px-3 py-4 text-xs text-slate-400">还没有会话</li>
        )}
        {sessions.map((s) => (
          <li
            key={s.id}
            className={cn(
              'group flex cursor-pointer items-center justify-between gap-1 px-3 py-2 hover:bg-slate-50',
              selectedID === s.id && 'bg-slate-100',
            )}
            onClick={() => onSelect(s.id)}
          >
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm font-medium text-slate-800">
                {s.title}
              </div>
              <div className="mt-0.5 truncate text-[11px] text-slate-400">
                {new Date(s.updated_at).toLocaleString()}
              </div>
            </div>
            <div className="hidden gap-1 group-hover:flex">
              <button
                type="button"
                className="rounded p-1 text-slate-400 hover:bg-slate-200 hover:text-slate-700"
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
                className="rounded p-1 text-slate-400 hover:bg-red-100 hover:text-red-600"
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
    <div className="grid gap-3 border-b border-slate-200 p-3 sm:grid-cols-2">
      <div className="space-y-1">
        <Label htmlFor="ai-environment">环境</Label>
        <select
          id="ai-environment"
          value={envID}
          onChange={(event) => onEnvChange(event.target.value)}
          className="h-9 w-full rounded-md border border-slate-300 bg-white px-3 text-sm outline-none focus:border-slate-500"
        >
          <option value="">请选择环境</option>
          {environments.map((env) => (
            <option key={env.id} value={env.id}>
              {env.name}
            </option>
          ))}
        </select>
      </div>
      <div className="space-y-1">
        <Label htmlFor="ai-ssh-config">SSH 配置</Label>
        <select
          id="ai-ssh-config"
          value={configID}
          onChange={(event) => onConfigChange(event.target.value)}
          disabled={!envID}
          className="h-9 w-full rounded-md border border-slate-300 bg-white px-3 text-sm outline-none focus:border-slate-500 disabled:bg-slate-50"
        >
          <option value="">请选择 SSH 配置</option>
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
  const config = env?.configs.find((c) => c.id === session.config_id);
  return (
    <div className="flex items-center justify-between border-b border-slate-200 px-3 py-2 text-xs text-slate-500">
      <div className="truncate">
        <span className="text-slate-600">{session.title}</span>
        <span className="mx-2 text-slate-300">·</span>
        环境 {env?.name ?? session.environment_id}
        <span className="mx-1 text-slate-300">/</span>
        SSH {config?.name ?? session.config_id}
      </div>
      {session.context_prefetched && (
        <span className="rounded-sm bg-emerald-50 px-1.5 py-0.5 text-[10px] text-emerald-600">
          已采集服务器信息
        </span>
      )}
    </div>
  );
}

// MessageList 把会话历史与当前 pending 轮次合并渲染。
function MessageList({
  session,
  pending,
  onOpenWorkflow,
}: {
  session: AISession | undefined;
  pending: PendingTurn | null;
  onOpenWorkflow: (workflowID: string) => void;
}) {
  const visible = useMemo(() => {
    if (!session) return [];
    return session.messages.filter(
      (m) => !m.hidden && (m.role === 'user' || m.role === 'assistant'),
    );
  }, [session]);

  const showPending = pending && pending.sessionID === (session?.id ?? '');

  if (visible.length === 0 && !showPending) {
    return (
      <div className="text-sm text-slate-500">
        选择左侧会话继续对话，或输入新需求。例如「分析一下这台服务器目前的状态」。
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {visible.map((message) => (
        <Bubble
          key={message.id}
          message={message}
          onOpenWorkflow={onOpenWorkflow}
        />
      ))}
      {showPending && pending && (
        <>
          <Bubble
            message={{
              id: pending.requestID + ':user',
              role: 'user',
              content: pending.userContent,
              created_at: new Date().toISOString(),
            }}
            onOpenWorkflow={onOpenWorkflow}
          />
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
              created_at: new Date().toISOString(),
            }}
            onOpenWorkflow={onOpenWorkflow}
          />
        </>
      )}
    </div>
  );
}

// Bubble 渲染单条会话消息气泡。
function Bubble({
  message,
  onOpenWorkflow,
}: {
  message: AISessionMessage;
  onOpenWorkflow: (workflowID: string) => void;
}) {
  const isUser = message.role === 'user';
  return (
    <div className={isUser ? 'flex justify-end' : 'flex justify-start'}>
      <div
        className={cn(
          'max-w-[82%] rounded-md px-3 py-2 text-sm leading-6',
          isUser
            ? 'bg-slate-900 text-white'
            : 'border border-slate-200 bg-white text-slate-700',
        )}
      >
        {message.content && (
          <div className="whitespace-pre-wrap">{message.content}</div>
        )}
        {!message.content && !isUser && (
          <div className="text-slate-400">正在处理…</div>
        )}
        {message.progress && message.progress.length > 0 && (
          <ol className="mt-2 space-y-1 border-t border-slate-200 pt-2 text-xs text-slate-500">
            {message.progress.map((item, index) => (
              <li key={`${item}-${index}`}>{item}</li>
            ))}
          </ol>
        )}
        {message.workflow_id && (
          <div className="mt-3 flex items-center justify-between gap-3 rounded-md border border-slate-200 bg-slate-50 px-3 py-2">
            <span className="truncate text-xs text-slate-600">
              {message.workflow_name || 'AI 生成工作流'}
            </span>
            <Button
              type="button"
              size="sm"
              onClick={() => onOpenWorkflow(message.workflow_id!)}
            >
              打开工作流
            </Button>
          </div>
        )}
      </div>
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
