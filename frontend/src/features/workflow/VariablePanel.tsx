// 变量/参数/返回值管理面板（左侧栏）
// 支持新增、删除、编辑；同时可作为「参数」、「返回值」、「变量」三种用途
// 点击列表项进入编辑模式

import { useState } from 'react';
import { Input } from '@/components/ui/Input';
import { Button } from '@/components/ui/Button';
import { cn } from '@/lib/cn';
import { getPortColor } from '@/types/nodeType';
import { setDragPayload, type DragNodePayload } from './dragNode';

// 列表项的最小结构
export interface VarItem {
  name: string;
  var_type: string;
  default?: unknown;
}

interface Props<T extends VarItem> {
  title: string;
  items: T[];
  onChange: (items: T[]) => void;
  // 是否显示「默认值」字段（参数/返回值无默认值，变量有）
  showDefault?: boolean;
  // 默认新建项的工厂（便于扩展自定义字段）
  factory?: () => T;
  // 拖拽到画布的 payload 生成器；返回 null 表示该面板不可拖拽
  dragPayload?: (item: T) => DragNodePayload | null;
}

// 类型选项（与后端 PortType 一致，排除 Exec/Dynamic）
const TYPE_OPTIONS = [
  'String',
  'Int',
  'Float',
  'Bool',
  'LinuxSshConnection',
  'LinuxFileHandle',
  'DockerContext',
  'K8sContext',
  'NginxInstance',
];

export function VariablePanel<T extends VarItem>({
  title,
  items,
  onChange,
  showDefault = false,
  factory,
  dragPayload,
}: Props<T>) {
  const [adding, setAdding] = useState(false);
  const [editingIdx, setEditingIdx] = useState<number | null>(null);

  function handleAdd(item: T) {
    onChange([...items, item]);
    setAdding(false);
  }

  function handleUpdate(idx: number, item: T) {
    const next = [...items];
    next[idx] = item;
    onChange(next);
    setEditingIdx(null);
  }

  function handleDelete(index: number) {
    onChange(items.filter((_, i) => i !== index));
    if (editingIdx === index) setEditingIdx(null);
  }

  return (
    <section className="border-b border-slate-200">
      <header className="flex items-center justify-between px-3 py-2">
        <h3 className="text-[11px] font-semibold uppercase tracking-wide text-slate-500">
          {title}
        </h3>
        {!adding && (
          <button
            type="button"
            onClick={() => {
              setAdding(true);
              setEditingIdx(null);
            }}
            className="text-xs text-blue-600 hover:text-blue-800"
          >
            + 添加
          </button>
        )}
      </header>

      <ul className="px-2 pb-2">
        {items.map((item, idx) => {
          if (editingIdx === idx) {
            return (
              <ItemForm
                key={`edit-${idx}`}
                initial={item}
                showDefault={showDefault}
                existingNames={items
                  .filter((_, i) => i !== idx)
                  .map((i) => i.name)}
                factory={factory}
                onCancel={() => setEditingIdx(null)}
                onSubmit={(updated) => handleUpdate(idx, updated)}
              />
            );
          }
          const payload = dragPayload ? dragPayload(item) : null;
          const draggable = !!payload;
          return (
            <li
              key={`${item.name}-${idx}`}
              draggable={draggable}
              onDragStart={(e) => {
                if (payload) setDragPayload(e, payload);
              }}
              onClick={(e) => {
                // 点击删除按钮区域 → 不进入编辑（含 visibility 隐藏时占位区域）
                const el =
                  e.target instanceof Element
                    ? e.target
                    : (e.target as Node).parentElement;
                if (el?.closest('[data-var-delete]')) return;
                setEditingIdx(idx);
                setAdding(false);
              }}
              className={cn(
                'group flex items-center gap-2 rounded px-1.5 py-1 hover:bg-slate-50 cursor-pointer',
                draggable && 'cursor-grab active:cursor-grabbing',
              )}
              title={draggable ? '点击编辑 / 拖到画布添加节点' : '点击编辑'}
            >
              <span
                className="inline-block size-2.5 rounded-full"
                style={{ background: getPortColor(item.var_type) }}
                title={item.var_type}
              />
              <span className="flex-1 truncate text-xs text-slate-800">
                {item.name}
              </span>
              <span className="text-[10px] text-slate-400">{item.var_type}</span>
              <button
                type="button"
                data-var-delete
                onMouseDown={(e) => e.stopPropagation()}
                onClick={(e) => {
                  e.stopPropagation();
                  handleDelete(idx);
                }}
                className="shrink-0 rounded p-0.5 text-xs leading-none text-red-500 opacity-40 hover:bg-red-50 hover:opacity-100 group-hover:opacity-100"
                title="删除"
              >
                ✕
              </button>
            </li>
          );
        })}
        {items.length === 0 && !adding && (
          <li className="px-1.5 py-1 text-[11px] text-slate-400">（空）</li>
        )}

        {adding && (
          <ItemForm
            showDefault={showDefault}
            existingNames={items.map((i) => i.name)}
            factory={factory}
            onCancel={() => setAdding(false)}
            onSubmit={handleAdd}
          />
        )}
      </ul>
    </section>
  );
}

// 内联表单：支持「新增」（initial 为 undefined）和「编辑」（initial 传入当前值）两种模式
function ItemForm<T extends VarItem>({
  initial,
  showDefault,
  existingNames,
  factory,
  onSubmit,
  onCancel,
}: {
  initial?: T;
  showDefault: boolean;
  existingNames: string[];
  factory?: () => T;
  onSubmit: (item: T) => void;
  onCancel: () => void;
}) {
  const [name, setName] = useState(initial?.name ?? '');
  const [varType, setVarType] = useState(initial?.var_type ?? 'String');
  const [defaultVal, setDefaultVal] = useState(
    typeof initial?.default === 'string' ? initial.default : '',
  );
  const [error, setError] = useState<string | null>(null);

  function handleSubmit() {
    const trimmed = name.trim();
    if (!trimmed) {
      setError('名称不能为空');
      return;
    }
    if (existingNames.includes(trimmed)) {
      setError('名称已存在');
      return;
    }
    const base: VarItem = { name: trimmed, var_type: varType };
    if (showDefault && defaultVal.trim()) {
      base.default = defaultVal.trim();
    }
    const merged = factory
      ? { ...factory(), ...base }
      : { ...(initial ?? {}), ...base };
    onSubmit(merged as T);
  }

  return (
    <li className="mt-1 space-y-1.5 rounded border border-slate-200 bg-slate-50 p-2">
      <Input
        placeholder="名称"
        value={name}
        onChange={(e) => setName(e.target.value)}
        autoFocus
        onKeyDown={(e) => {
          if (e.key === 'Enter') handleSubmit();
          if (e.key === 'Escape') onCancel();
        }}
      />
      <select
        value={varType}
        onChange={(e) => setVarType(e.target.value)}
        className={cn(
          'w-full rounded border border-slate-300 bg-white px-2 py-1 text-xs',
        )}
      >
        {TYPE_OPTIONS.map((t) => (
          <option key={t} value={t}>
            {t}
          </option>
        ))}
      </select>
      {showDefault && (
        <Input
          placeholder="默认值（可选）"
          value={defaultVal}
          onChange={(e) => setDefaultVal(e.target.value)}
        />
      )}
      {error && (
        <div className="text-[11px] text-red-600">{error}</div>
      )}
      <div className="flex gap-1">
        <Button size="sm" onClick={handleSubmit} className="flex-1">
          {initial ? '保存' : '确定'}
        </Button>
        <Button size="sm" variant="secondary" onClick={onCancel}>
          取消
        </Button>
      </div>
    </li>
  );
}
