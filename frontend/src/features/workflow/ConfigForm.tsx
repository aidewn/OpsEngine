// 节点配置表单：根据 ConfigSchema 动态渲染
// 字段类型：text / password / number / select / toggle / textarea
//
// 数据流：
//   - 内部 local state（避免外部 controlled 模式下每次按键都触发 onChange）
//   - 500ms debounce 后调 props.onChange
//   - 切换节点（value prop 变化）→ 重置 local state
//   - 组件 unmount 时立即 flush 未保存的修改

import { useEffect, useRef, useState } from 'react';
import { Input } from '@/components/ui/Input';
import { Textarea } from '@/components/ui/Textarea';
import { Label } from '@/components/ui/Label';
import { cn } from '@/lib/cn';
import type { FieldSchema } from '@/types/nodeType';

interface Props {
  schema: FieldSchema[];
  value: Record<string, unknown>;
  onChange: (next: Record<string, unknown>) => void;
}

export function ConfigForm({ schema, value, onChange }: Props) {
  const [local, setLocal] = useState<Record<string, unknown>>(value);
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;
  const lastFlushedRef = useRef(value);
  // unmount cleanup 必须读最新 local；空依赖 effect 里的 local 会停在首次渲染的值
  const localRef = useRef(local);
  localRef.current = local;

  // 外部 value 同步策略：
  // - 仅当与上次 debounce flush 的内容一致时才采纳（服务端/缓存回显）
  // - 避免 mutate 尚未更新 query 时，旧 workflow 引用触发 effect 把 local 打回旧 config
  // 切换节点由 ConfigTab 的 key={instance_id} 卸载重建，不在此处理
  const valueKey = JSON.stringify(value);
  useEffect(() => {
    if (shallowEq(value, lastFlushedRef.current)) {
      setLocal(value);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [valueKey]);

  // 500ms debounce flush
  useEffect(() => {
    if (shallowEq(local, lastFlushedRef.current)) return;
    const timer = setTimeout(() => {
      onChangeRef.current(local);
      lastFlushedRef.current = local;
    }, 500);
    return () => clearTimeout(timer);
  }, [local]);

  // 失焦/切节点/点画布取消选中时会卸载：flush 尚未 debounce 的修改
  useEffect(() => {
    return () => {
      const pending = localRef.current;
      if (!shallowEq(pending, lastFlushedRef.current)) {
        onChangeRef.current(pending);
        lastFlushedRef.current = pending;
      }
    };
  }, []);

  if (schema.length === 0) {
    return <div className="text-xs text-slate-400">（无配置项）</div>;
  }

  return (
    <div className="space-y-3">
      {schema.map((field) => (
        <FieldRow
          key={field.id}
          field={field}
          value={local[field.id]}
          onChange={(v) =>
            setLocal((prev) => ({ ...prev, [field.id]: v }))
          }
        />
      ))}
    </div>
  );
}

// 单字段渲染
function FieldRow({
  field,
  value,
  onChange,
}: {
  field: FieldSchema;
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  return (
    <div className="space-y-1">
      <Label htmlFor={field.id} className="text-xs">
        {field.label}
        {field.required && <span className="ml-0.5 text-red-500">*</span>}
      </Label>
      <FieldControl field={field} value={value} onChange={onChange} />
    </div>
  );
}

// 不同 type 的控件
function FieldControl({
  field,
  value,
  onChange,
}: {
  field: FieldSchema;
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  switch (field.type) {
    case 'textarea':
      return (
        <Textarea
          id={field.id}
          value={(value as string) ?? ''}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.placeholder}
          rows={4}
        />
      );
    case 'password':
      return (
        <Input
          id={field.id}
          type="password"
          value={(value as string) ?? ''}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.placeholder}
        />
      );
    case 'number':
      return (
        <Input
          id={field.id}
          type="number"
          value={value === undefined || value === null ? '' : String(value)}
          onChange={(e) => {
            const raw = e.target.value;
            if (raw === '') {
              onChange(undefined);
              return;
            }
            const n = Number(raw);
            if (Number.isNaN(n)) return;
            onChange(n);
          }}
          placeholder={field.placeholder}
          min={field.min !== undefined ? field.min : undefined}
          max={field.max !== undefined ? field.max : undefined}
        />
      );
    case 'select':
      return (
        <select
          id={field.id}
          value={(value as string) ?? ''}
          onChange={(e) => onChange(e.target.value)}
          className={cn(
            'w-full rounded border border-slate-300 bg-white px-2 py-1.5 text-xs',
          )}
        >
          {field.options?.map((opt) => (
            <option key={opt} value={opt}>
              {opt}
            </option>
          ))}
        </select>
      );
    case 'toggle':
      return (
        <label className="inline-flex cursor-pointer items-center gap-2">
          <input
            id={field.id}
            type="checkbox"
            checked={!!value}
            onChange={(e) => onChange(e.target.checked)}
            className="size-4 rounded border-slate-300"
          />
          <span className="text-xs text-slate-600">
            {value ? '已启用' : '已禁用'}
          </span>
        </label>
      );
    case 'text':
    default:
      return (
        <Input
          id={field.id}
          type="text"
          value={(value as string) ?? ''}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.placeholder}
        />
      );
  }
}

// 浅相等判断（map 顶层 key 一一对比）
function shallowEq(a: Record<string, unknown>, b: Record<string, unknown>): boolean {
  const ak = Object.keys(a);
  const bk = Object.keys(b);
  if (ak.length !== bk.length) return false;
  for (const k of ak) {
    if (a[k] !== b[k]) return false;
  }
  return true;
}
