// CommandPalette 组件：全局命令面板，支持跳转和向 AI 助手预填问题。
import { Command } from 'cmdk';
import { Title as DialogTitle } from '@radix-ui/react-dialog';
import type { ReactNode } from 'react';
import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Bot,
  FileText,
  GitBranch,
  History,
  Layers,
  MessageSquare,
  Settings,
} from 'lucide-react';
import { useAISessions } from '@/api/ai';
import { useAssembles } from '@/api/assembles';
import { useExecutions } from '@/api/executions';
import { useOpsDocs } from '@/api/opsDocs';
import { useWorkflows } from '@/api/workflows';
import { Dialog } from '@/components/ui/Dialog';
import { AIAssistantDialog } from '@/features/ai/AIAssistantDialog';

export const COMMAND_PALETTE_EVENT = 'ops:command-palette';

export function CommandPalette() {
  const navigate = useNavigate();
  const { data: workflows } = useWorkflows();
  const { data: assembles } = useAssembles();
  const { data: executions } = useExecutions();
  const { data: docs } = useOpsDocs();
  const { data: sessions } = useAISessions();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [aiOpen, setAIOpen] = useState(false);
  const [aiDraft, setAIDraft] = useState('');
  const [aiFocusInput, setAIFocusInput] = useState(false);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        setOpen((value) => !value);
        return;
      }
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'i') {
        event.preventDefault();
        navigate('/?tab=chat&focus=ai');
        setAIDraft('');
        return;
      }
      if (event.key === '?' && !event.ctrlKey && !event.metaKey && !event.altKey && !isEditableTarget(event.target)) {
        event.preventDefault();
        setShortcutsOpen(true);
      }
    };
    const handleOpen = () => setOpen(true);

    window.addEventListener('keydown', handleKeyDown);
    window.addEventListener(COMMAND_PALETTE_EVENT, handleOpen);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
      window.removeEventListener(COMMAND_PALETTE_EVENT, handleOpen);
    };
  }, [navigate]);

  const trimmedQuery = query.trim();
  const recentSessions = useMemo(
    () => [...(sessions ?? [])].sort((a, b) => b.updated_at.localeCompare(a.updated_at)).slice(0, 6),
    [sessions],
  );

  function run(command: () => void) {
    command();
    setOpen(false);
    setQuery('');
  }

  return (
    <>
      <Command.Dialog
        open={open}
        onOpenChange={setOpen}
        label="命令面板"
        className="fixed left-1/2 top-24 z-[80] w-[min(720px,calc(100vw-32px))] -translate-x-1/2 overflow-hidden rounded-lg border border-ops-border-subtle bg-ops-elevated text-ops-primary shadow-2xl"
        overlayClassName="fixed inset-0 z-[70] bg-black/50"
      >
        <DialogTitle className="sr-only">命令面板</DialogTitle>
        <Command.Input
          value={query}
          onValueChange={setQuery}
          placeholder="搜索或向 AI 提问…"
          className="h-12 w-full border-b border-ops-border-subtle bg-ops-input px-4 text-sm text-ops-primary outline-none placeholder:text-ops-tertiary"
        />
        <Command.List className="max-h-[520px] overflow-y-auto p-2">
          <Command.Empty className="px-3 py-8 text-center text-sm text-ops-secondary">没有匹配结果</Command.Empty>

          {trimmedQuery ? (
            <Command.Group heading="AI" className="command-group">
              <CommandItem
                icon={<Bot size={16} />}
                label={`向 AI 提问：${trimmedQuery}`}
                onSelect={() =>
                  run(() => {
                    setAIDraft(trimmedQuery);
                    setAIFocusInput(true);
                    setAIOpen(true);
                  })
                }
              />
            </Command.Group>
          ) : null}

          <Command.Group heading="跳转" className="command-group">
            <CommandItem icon={<MessageSquare size={16} />} label="Chat" onSelect={() => run(() => navigate('/?tab=chat'))} />
            <CommandItem icon={<GitBranch size={16} />} label="工作流" onSelect={() => run(() => navigate('/?tab=workflow'))} />
            <CommandItem icon={<FileText size={16} />} label="报告" onSelect={() => run(() => navigate('/?tab=reports'))} />
            <CommandItem icon={<Settings size={16} />} label="环境配置" onSelect={() => run(() => navigate('/settings/environments'))} />
          </Command.Group>

          <Command.Group heading="工作流" className="command-group">
            {(workflows ?? []).map((workflow) => (
              <CommandItem
                key={workflow.id}
                icon={<GitBranch size={16} />}
                label={workflow.name}
                detail={workflow.description || workflow.id}
                onSelect={() => run(() => navigate(`/workflows/${workflow.id}`))}
              />
            ))}
          </Command.Group>

          <Command.Group heading="集合" className="command-group">
            {(assembles ?? []).map((assemble) => (
              <CommandItem
                key={assemble.id}
                icon={<Layers size={16} />}
                label={assemble.name}
                detail={assemble.description || assemble.id}
                onSelect={() => run(() => navigate(`/assembles/${assemble.id}`))}
              />
            ))}
          </Command.Group>

          <Command.Group heading="执行" className="command-group">
            {(executions ?? []).slice(0, 12).map((execution) => (
              <CommandItem
                key={execution.id}
                icon={<History size={16} />}
                label={`${execution.workflow_name} #${execution.id.slice(0, 6)}`}
                detail={execution.status}
                onSelect={() => run(() => navigate(`/executions/${execution.id}`))}
              />
            ))}
          </Command.Group>

          <Command.Group heading="文档" className="command-group">
            {(docs ?? []).slice(0, 12).map((doc) => (
              <CommandItem
                key={doc.id}
                icon={<FileText size={16} />}
                label={doc.title}
                detail={doc.kind}
                onSelect={() => run(() => navigate(`/?tab=reports&doc=${doc.id}`))}
              />
            ))}
          </Command.Group>

          <Command.Group heading="最近会话" className="command-group">
            {recentSessions.map((session) => (
              <CommandItem
                key={session.id}
                icon={<MessageSquare size={16} />}
                label={session.title}
                detail={new Date(session.updated_at).toLocaleString()}
                onSelect={() => run(() => navigate(`/?tab=chat&session=${session.id}`))}
              />
            ))}
          </Command.Group>
        </Command.List>
      </Command.Dialog>
      <AIAssistantDialog
        open={aiOpen}
        onOpenChange={(next) => {
          setAIOpen(next);
          if (!next) setAIFocusInput(false);
        }}
        initialMessage={aiDraft}
        focusInputOnOpen={aiFocusInput}
      />
      <ShortcutsDialog open={shortcutsOpen} onOpenChange={setShortcutsOpen} />
    </>
  );
}

// isEditableTarget 避免用户在输入框内输入 ? 时被全局快捷键截获。
function isEditableTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false;
  const tagName = target.tagName.toLowerCase();
  return target.isContentEditable || tagName === 'input' || tagName === 'textarea' || tagName === 'select';
}

function CommandItem({
  icon,
  label,
  detail,
  onSelect,
}: {
  icon: ReactNode;
  label: string;
  detail?: string;
  onSelect: () => void;
}) {
  return (
    <Command.Item
      value={`${label} ${detail ?? ''}`}
      onSelect={onSelect}
      className="flex cursor-default items-center gap-3 rounded-md px-3 py-2 text-sm outline-none data-[selected=true]:bg-ops-surface"
    >
      <span className="text-ops-secondary">{icon}</span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-ops-primary">{label}</span>
        {detail ? <span className="block truncate text-2xs text-ops-tertiary">{detail}</span> : null}
      </span>
    </Command.Item>
  );
}

function ShortcutsDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title="快捷键" size="sm">
      <div className="space-y-3 text-sm text-ops-secondary">
        <ShortcutRow keys="Ctrl/Cmd + K" label="打开命令面板" />
        <ShortcutRow keys="Ctrl/Cmd + I" label="打开 Chat 并聚焦 AI 输入" />
        <ShortcutRow keys="Ctrl/Cmd + Enter" label="发送 AI 消息" />
        <ShortcutRow keys="?" label="查看快捷键" />
      </div>
    </Dialog>
  );
}

function ShortcutRow({ keys, label }: { keys: string; label: string }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span>{label}</span>
      <kbd className="rounded border border-ops-border-subtle bg-ops-input px-2 py-1 font-mono text-2xs text-ops-primary">
        {keys}
      </kbd>
    </div>
  );
}
