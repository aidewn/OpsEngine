// Composer：AI 对话输入栏。把环境/SSH 选择收进底部 chip，输入 / 弹出模式菜单。
// 状态全部由父组件持有（受控），本组件只负责呈现与交互编排。
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import {
  ArrowUp,
  Server,
  Box,
  X,
  Check,
  Slash,
  AtSign,
  ChevronDown,
  Workflow,
  ListChecks,
  MessagesSquare,
  Stethoscope,
  ClipboardCheck,
  Network,
  GitBranch,
} from 'lucide-react';
import { Textarea } from '@/components/ui/Textarea';
import { cn } from '@/lib/cn';
import type { EnvironmentDef, EnvConfigItem } from '@/types/environment';
import { matchModes, type ChatMode, type ChatModeGroup } from './chatModes';

// 模式图标映射（chatModes 里存图标名，这里解析为组件）
const MODE_ICONS: Record<string, typeof Workflow> = {
  Workflow,
  ListChecks,
  MessagesSquare,
  Stethoscope,
  ClipboardCheck,
  Network,
  GitBranch,
};

interface ComposerProps {
  value: string;
  onChange: (v: string) => void;
  onSubmit: () => void;
  disabled?: boolean;
  placeholder?: string;
  inputRef?: React.RefObject<HTMLTextAreaElement | null>;
  // 环境/SSH 上下文（chip 展示与选择）
  environments: EnvironmentDef[];
  envID: string;
  configID: string;
  sshConfigs: EnvConfigItem[];
  onEnvChange: (id: string) => void;
  onConfigChange: (id: string) => void;
  // 模式 chip：能力可叠加，流程独占；toggle 切换进/出
  activeModes: ChatMode[];
  onToggleMode: (m: ChatMode) => void;
}

export function Composer({
  value,
  onChange,
  onSubmit,
  disabled,
  placeholder,
  inputRef,
  environments,
  envID,
  configID,
  sshConfigs,
  onEnvChange,
  onConfigChange,
  activeModes,
  onToggleMode,
}: ComposerProps) {
  // 双触发符：/ → 流程（route，单选），@ → 能力（capability，可叠加）。
  const trigger: ChatModeGroup | null = value.startsWith('/')
    ? 'route'
    : value.startsWith('@')
      ? 'capability'
      : null;
  const menuQuery = trigger ? value.slice(1) : '';
  const modeMenuOpen = trigger !== null;
  const matched = useMemo(
    () => (trigger ? matchModes(menuQuery, trigger) : []),
    [trigger, menuQuery],
  );
  const [menuIdx, setMenuIdx] = useState(0);
  useEffect(() => setMenuIdx(0), [value]);

  const envName = environments.find((e) => e.id === envID)?.name;
  const configName = sshConfigs.find((c) => c.id === configID)?.name;

  // 选中模式：切换进/出后清空 / 文本，菜单关闭；能力可继续按 / 再加
  function pickMode(m: ChatMode) {
    onToggleMode(m);
    onChange('');
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (modeMenuOpen && matched.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setMenuIdx((i) => (i + 1) % matched.length);
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setMenuIdx((i) => (i - 1 + matched.length) % matched.length);
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault();
        pickMode(matched[menuIdx]!);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        onChange('');
        return;
      }
    }
    // 退格删空：若已有模式 chip 且输入为空，退格删最后一个 chip
    if (e.key === 'Backspace' && value === '' && activeModes.length > 0) {
      e.preventDefault();
      onToggleMode(activeModes[activeModes.length - 1]!);
      return;
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      if (!e.nativeEvent.isComposing && !modeMenuOpen) onSubmit();
    }
  }

  return (
    <div className="relative">
      {/* 模式菜单：/ 流程（单选） · @ 能力（可叠加） */}
      {modeMenuOpen && (
        <div className="absolute bottom-full left-0 mb-2 w-[340px] overflow-hidden rounded-lg border border-ops-border-subtle bg-ops-elevated shadow-2xl data-[open]:animate-in" data-open>
          <div className="flex items-center justify-between border-b border-ops-border-subtle px-3 py-1.5 text-2xs text-ops-tertiary">
            <span className="uppercase">{trigger === 'route' ? '流程模式 · 单选' : '能力 · 可叠加'}</span>
            {menuQuery ? <span className="truncate">匹配 “{menuQuery}”</span> : null}
          </div>
          {matched.length === 0 ? (
            <div className="px-3 py-3 text-xs text-ops-tertiary">无匹配模式</div>
          ) : (
            <div className="max-h-72 overflow-y-auto p-1">
              {matched.map((m, i) => {
                const Icon = MODE_ICONS[m.icon] ?? Workflow;
                const selected = activeModes.some((x) => x.id === m.id);
                return (
                  <button
                    key={m.id}
                    type="button"
                    onMouseEnter={() => setMenuIdx(i)}
                    onClick={() => pickMode(m)}
                    className={cn(
                      'flex w-full items-center gap-2.5 rounded px-2.5 py-2 text-left transition-colors duration-fast ease-ops',
                      i === menuIdx ? 'bg-ops-surface' : '',
                    )}
                  >
                    <Icon size={16} className={cn('shrink-0', selected ? 'text-ops-accent' : 'text-ops-tertiary')} />
                    <span className="w-20 shrink-0 truncate text-xs font-medium text-ops-primary">{m.label}</span>
                    <span className="min-w-0 flex-1 truncate text-2xs text-ops-tertiary">{m.desc}</span>
                    {selected && <Check size={14} className="shrink-0 text-ops-accent" />}
                  </button>
                );
              })}
            </div>
          )}
        </div>
      )}

      <div className="overflow-hidden rounded-xl border border-ops-border-subtle bg-ops-surface transition-colors duration-fast ease-ops focus-within:border-ops-border-strong">
        <Textarea
          ref={inputRef as React.Ref<HTMLTextAreaElement>}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          rows={1}
          placeholder={
            activeModes.length > 0
              ? `${activeModes.map((m) => m.label).join(' + ')} · 输入内容…`
              : placeholder
          }
          disabled={disabled}
          onKeyDown={handleKeyDown}
          className="max-h-40 min-h-[40px] resize-none border-0 bg-transparent px-3 py-2.5 focus:ring-0"
        />
        <div className="flex items-center justify-between gap-2 px-2 pb-2">
          <div className="flex min-w-0 items-center gap-1.5">
            {/* 环境 chip */}
            <ChipSelect
              icon={<Box size={13} />}
              label={envName ? `环境 ${envName}` : '选择环境'}
              active={!!envID}
              options={environments.map((e) => ({ id: e.id, name: e.name }))}
              value={envID}
              emptyHint="（不指定环境）"
              allowEmpty
              onSelect={onEnvChange}
            />
            {/* SSH chip：仅选了环境时可用 */}
            <ChipSelect
              icon={<Server size={13} />}
              label={configName ? configName : '不指定 SSH'}
              active={!!configID}
              disabled={!envID}
              options={sshConfigs.map((c) => ({ id: c.id, name: c.name }))}
              value={configID}
              emptyHint="不指定（环境级会话）"
              allowEmpty
              onSelect={onConfigChange}
            />
            {/* 模式 chip（可叠加多个，各自可删） */}
            {activeModes.map((m) => (
              <span
                key={m.id}
                className="inline-flex h-7 items-center gap-1 rounded-lg border border-ops-accent/40 bg-ops-accent-soft px-2 text-2xs text-ops-accent"
              >
                {m.label}
                <button
                  type="button"
                  onClick={() => onToggleMode(m)}
                  className="text-ops-accent/70 hover:text-ops-accent"
                  aria-label={`取消${m.label}`}
                >
                  <X size={12} />
                </button>
              </span>
            ))}
            {/* 双触发提示：/ 流程 · @ 能力 */}
            {!modeMenuOpen && (
              <div className="flex items-center gap-0.5">
                <button
                  type="button"
                  onClick={() => onChange('/')}
                  className="inline-flex h-7 items-center gap-0.5 rounded-lg px-1.5 text-2xs text-ops-tertiary transition-colors duration-fast ease-ops hover:text-ops-secondary"
                  title="/ 选择流程（单选）"
                >
                  <Slash size={12} />
                </button>
                <button
                  type="button"
                  onClick={() => onChange('@')}
                  className="inline-flex h-7 items-center gap-0.5 rounded-lg px-1.5 text-2xs text-ops-tertiary transition-colors duration-fast ease-ops hover:text-ops-secondary"
                  title="@ 添加能力（可叠加）"
                >
                  <AtSign size={12} />
                </button>
              </div>
            )}
          </div>
          <button
            type="button"
            onClick={onSubmit}
            disabled={disabled || !value.trim()}
            className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-ops-accent text-ops-inverse transition-colors duration-fast ease-ops hover:bg-ops-accent-hover disabled:bg-ops-border-subtle disabled:text-ops-tertiary"
            aria-label="发送"
          >
            <ArrowUp size={16} />
          </button>
        </div>
      </div>
    </div>
  );
}

// ChipSelect：紧凑下拉 chip，点击展开浮层选项。
function ChipSelect({
  icon,
  label,
  active,
  disabled,
  options,
  value,
  emptyHint,
  allowEmpty,
  onSelect,
}: {
  icon: ReactNode;
  label: string;
  active: boolean;
  disabled?: boolean;
  options: { id: string; name: string }[];
  value: string;
  emptyHint: string;
  allowEmpty?: boolean;
  onSelect: (id: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, [open]);

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        disabled={disabled}
        onClick={() => setOpen((v) => !v)}
        className={cn(
          'inline-flex h-7 max-w-[160px] items-center gap-1.5 rounded-lg border px-2 text-2xs transition-colors duration-fast ease-ops',
          active
            ? 'border-ops-border-strong bg-ops-elevated text-ops-primary'
            : 'border-ops-border-subtle text-ops-secondary hover:text-ops-primary',
          disabled && 'cursor-not-allowed opacity-40',
        )}
      >
        <span className="shrink-0 text-ops-tertiary">{icon}</span>
        <span className="truncate">{label}</span>
        <ChevronDown size={11} className="shrink-0 text-ops-tertiary" />
      </button>
      {open && (
        <div className="absolute bottom-full left-0 mb-1.5 max-h-60 w-52 overflow-y-auto rounded-lg border border-ops-border-subtle bg-ops-elevated p-1 shadow-2xl">
          {allowEmpty && (
            <MenuRow
              label={emptyHint}
              selected={value === ''}
              onClick={() => {
                onSelect('');
                setOpen(false);
              }}
            />
          )}
          {options.length === 0 && !allowEmpty ? (
            <div className="px-2 py-2 text-2xs text-ops-tertiary">无可选项</div>
          ) : (
            options.map((o) => (
              <MenuRow
                key={o.id}
                label={o.name}
                selected={value === o.id}
                onClick={() => {
                  onSelect(o.id);
                  setOpen(false);
                }}
              />
            ))
          )}
        </div>
      )}
    </div>
  );
}

function MenuRow({ label, selected, onClick }: { label: string; selected: boolean; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'flex w-full items-center justify-between rounded px-2 py-1.5 text-left text-xs transition-colors duration-fast ease-ops hover:bg-ops-surface',
        selected ? 'text-ops-primary' : 'text-ops-secondary',
      )}
    >
      <span className="truncate">{label}</span>
      {selected && <span className="ml-2 text-ops-accent">·</span>}
    </button>
  );
}
