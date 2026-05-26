// 节点配置表单：根据 ConfigSchema 动态渲染
// 字段类型：text / password / number / select / toggle / textarea / variable_select
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
import type { ParamDef } from '@/types/assemble';
import type { VariableDef } from '@/types/workflow';
import { useEnvironments } from '@/api/environments';
import type { EnvConfigItem, EnvConfigKind } from '@/types/environment';

interface Props {
  schema: FieldSchema[];
  value: Record<string, unknown>;
  onChange: (next: Record<string, unknown>) => void;
  // 当前图已定义变量；variable_select 字段从中取选项
  variables?: VariableDef[];
  // 集合 params / returns；param_select / return_select 从中取选项
  params?: ParamDef[];
  returns?: ParamDef[];
}

export function ConfigForm({
  schema,
  value,
  onChange,
  variables,
  params,
  returns,
}: Props) {
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

  // 单 key 更新（多数字段）
  const setOne = (id: string, v: unknown) =>
    setLocal((prev) => ({ ...prev, [id]: v }));
  // 多 key 合并（如 variable_select 同时写 var_name + var_type）
  const setPatch = (patch: Record<string, unknown>) =>
    setLocal((prev) => ({ ...prev, ...patch }));

  return (
    <div className="space-y-3">
      {schema.map((field) => (
        <FieldRow
          key={field.id}
          field={field}
          value={local[field.id]}
          formValue={local}
          onChange={(v) => setOne(field.id, v)}
          onPatch={setPatch}
          variables={variables}
          params={params}
          returns={returns}
        />
      ))}
    </div>
  );
}

// 单字段渲染
function FieldRow({
  field,
  value,
  formValue,
  onChange,
  onPatch,
  variables,
  params,
  returns,
}: {
  field: FieldSchema;
  value: unknown;
  formValue: Record<string, unknown>;
  onChange: (v: unknown) => void;
  onPatch: (patch: Record<string, unknown>) => void;
  variables?: VariableDef[];
  params?: ParamDef[];
  returns?: ParamDef[];
}) {
  // toggle：标签在上、勾选框在下，与其他字段对齐
  if (field.type === 'toggle') {
    const checked = !!value;
    return (
      <div className="space-y-1.5">
        <Label htmlFor={field.id} className="text-xs">
          {field.label}
          {field.required && <span className="ml-0.5 text-red-500">*</span>}
        </Label>
        <label
          htmlFor={field.id}
          className="inline-flex cursor-pointer items-center gap-2.5"
        >
          <input
            id={field.id}
            type="checkbox"
            checked={checked}
            onChange={(e) => onChange(e.target.checked)}
            className="size-4 shrink-0 rounded border-slate-300 accent-blue-600"
          />
          <span className="text-xs leading-none text-slate-500">
            {checked ? '已启用' : '已禁用'}
          </span>
        </label>
      </div>
    );
  }

  return (
    <div className="space-y-1">
      <Label htmlFor={field.id} className="text-xs">
        {field.label}
        {field.required && <span className="ml-0.5 text-red-500">*</span>}
      </Label>
      <FieldControl
        field={field}
        value={value}
        formValue={formValue}
        onChange={onChange}
        onPatch={onPatch}
        variables={variables}
        params={params}
        returns={returns}
      />
    </div>
  );
}

// 不同 type 的控件
function FieldControl({
  field,
  value,
  formValue,
  onChange,
  onPatch,
  variables,
  params,
  returns,
}: {
  field: FieldSchema;
  value: unknown;
  formValue: Record<string, unknown>;
  onChange: (v: unknown) => void;
  onPatch: (patch: Record<string, unknown>) => void;
  variables?: VariableDef[];
  params?: ParamDef[];
  returns?: ParamDef[];
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
      // 由 FieldRow 整行渲染；此处不应走到
      return null;
    case 'variable_select':
      return (
        <VariableSelect
          id={field.id}
          value={(value as string) ?? ''}
          variables={variables ?? []}
          onPick={(v) =>
            onPatch({ var_name: v?.name ?? '', var_type: v?.var_type ?? '' })
          }
        />
      );
    case 'param_select':
      return (
        <ParamSelect
          id={field.id}
          value={(value as string) ?? ''}
          params={params ?? []}
          onPick={(p) =>
            onPatch({ param_name: p?.name ?? '', var_type: p?.var_type ?? '' })
          }
        />
      );
    case 'return_select':
      return (
        <ReturnSelect
          id={field.id}
          value={(value as string) ?? ''}
          returns={returns ?? []}
          onPick={(r) =>
            onPatch({
              return_name: r?.name ?? '',
              var_type: r?.var_type ?? '',
            })
          }
        />
      );
    case 'env_select':
      return (
        <EnvSelect
          id={field.id}
          value={(value as string) ?? ''}
          onPick={(envID) =>
            // 切换环境时清空 config_id，避免残留 stale 引用
            onPatch({ [field.id]: envID, config_id: '' })
          }
        />
      );
    case 'env_config_select':
      return (
        <EnvConfigSelect
          id={field.id}
          value={(value as string) ?? ''}
          envID={(formValue['environment_id'] as string) ?? ''}
          kindFilter={field.config_kind_filter as EnvConfigKind | undefined}
          onChange={onChange}
        />
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

// variable_select 控件：从当前图变量列表挑选；选中后同时写 var_name 与 var_type
// 引用未定义变量时仍保留原值，提示用户重新选择
function VariableSelect({
  id,
  value,
  variables,
  onPick,
}: {
  id: string;
  value: string;
  variables: VariableDef[];
  onPick: (v: VariableDef | null) => void;
}) {
  const exists = variables.some((v) => v.name === value);
  const isOrphan = value !== '' && !exists;

  if (variables.length === 0) {
    return (
      <div className="rounded border border-amber-300 bg-amber-50 px-2 py-1.5 text-xs text-amber-700">
        当前图没有定义任何变量，请先在左侧变量面板添加
      </div>
    );
  }

  return (
    <div className="space-y-1">
      <select
        id={id}
        value={value}
        onChange={(e) => {
          const picked = variables.find((v) => v.name === e.target.value);
          onPick(picked ?? null);
        }}
        className={cn(
          'w-full rounded border bg-white px-2 py-1.5 text-xs',
          isOrphan ? 'border-red-400' : 'border-slate-300',
        )}
      >
        <option value="">（请选择变量）</option>
        {isOrphan && (
          <option value={value}>未定义：{value}</option>
        )}
        {variables.map((v) => (
          <option key={v.name} value={v.name}>
            {v.name} ({v.var_type})
          </option>
        ))}
      </select>
      {isOrphan && (
        <div className="text-[11px] text-red-600">
          引用的变量已不存在，保存前请重新选择
        </div>
      )}
    </div>
  );
}

// param_select：从集合 Params 列表挑选；选中后写 param_name + var_type
function ParamSelect({
  id,
  value,
  params,
  onPick,
}: {
  id: string;
  value: string;
  params: ParamDef[];
  onPick: (p: ParamDef | null) => void;
}) {
  const exists = params.some((p) => p.name === value);
  const isOrphan = value !== '' && !exists;

  if (params.length === 0) {
    return (
      <div className="rounded border border-amber-300 bg-amber-50 px-2 py-1.5 text-xs text-amber-700">
        当前集合没有定义 Params，请先在 Start 节点或左侧 Params 面板添加
      </div>
    );
  }

  return (
    <div className="space-y-1">
      <select
        id={id}
        value={value}
        onChange={(e) => {
          const picked = params.find((p) => p.name === e.target.value);
          onPick(picked ?? null);
        }}
        className={cn(
          'w-full rounded border bg-white px-2 py-1.5 text-xs',
          isOrphan ? 'border-red-400' : 'border-slate-300',
        )}
      >
        <option value="">（请选择参数）</option>
        {isOrphan && <option value={value}>未定义：{value}</option>}
        {params.map((p) => (
          <option key={p.name} value={p.name}>
            {p.name} ({p.var_type})
          </option>
        ))}
      </select>
      {isOrphan && (
        <div className="text-[11px] text-red-600">
          引用的参数已不存在，保存前请重新选择
        </div>
      )}
    </div>
  );
}

// return_select：从集合 Returns 列表挑选；选中后写 return_name + var_type
function ReturnSelect({
  id,
  value,
  returns,
  onPick,
}: {
  id: string;
  value: string;
  returns: ParamDef[];
  onPick: (r: ParamDef | null) => void;
}) {
  const exists = returns.some((r) => r.name === value);
  const isOrphan = value !== '' && !exists;

  if (returns.length === 0) {
    return (
      <div className="rounded border border-amber-300 bg-amber-50 px-2 py-1.5 text-xs text-amber-700">
        当前集合没有定义 Returns，请先在 End 节点或左侧 Returns 面板添加
      </div>
    );
  }

  return (
    <div className="space-y-1">
      <select
        id={id}
        value={value}
        onChange={(e) => {
          const picked = returns.find((r) => r.name === e.target.value);
          onPick(picked ?? null);
        }}
        className={cn(
          'w-full rounded border bg-white px-2 py-1.5 text-xs',
          isOrphan ? 'border-red-400' : 'border-slate-300',
        )}
      >
        <option value="">（请选择返回值）</option>
        {isOrphan && <option value={value}>未定义：{value}</option>}
        {returns.map((r) => (
          <option key={r.name} value={r.name}>
            {r.name} ({r.var_type})
          </option>
        ))}
      </select>
      {isOrphan && (
        <div className="text-[11px] text-red-600">
          引用的返回值已不存在，保存前请重新选择
        </div>
      )}
    </div>
  );
}

// env_select 控件：列出所有环境；选中后 onPick(envID) 同时清空配套 config_id
function EnvSelect({
  id,
  value,
  onPick,
}: {
  id: string;
  value: string;
  onPick: (envID: string) => void;
}) {
  const { data, isLoading, error } = useEnvironments();
  if (isLoading) {
    return <div className="text-xs text-slate-400">加载环境列表...</div>;
  }
  if (error) {
    return (
      <div className="rounded border border-red-200 bg-red-50 px-2 py-1.5 text-xs text-red-700">
        加载环境失败：{error.message}
      </div>
    );
  }
  const envs = data ?? [];
  const exists = envs.some((e) => e.id === value);
  const isOrphan = value !== '' && !exists;

  if (envs.length === 0) {
    return (
      <div className="rounded border border-amber-300 bg-amber-50 px-2 py-1.5 text-xs text-amber-700">
        尚未创建任何环境，请先在首页「配置环境」Tab 添加
      </div>
    );
  }

  return (
    <div className="space-y-1">
      <select
        id={id}
        value={value}
        onChange={(e) => onPick(e.target.value)}
        className={cn(
          'w-full rounded border bg-white px-2 py-1.5 text-xs',
          isOrphan ? 'border-red-400' : 'border-slate-300',
        )}
      >
        <option value="">（请选择环境）</option>
        {isOrphan && <option value={value}>未定义：{value}</option>}
        {envs.map((e) => (
          <option key={e.id} value={e.id}>
            {e.name}
          </option>
        ))}
      </select>
      {isOrphan && (
        <div className="text-[11px] text-red-600">
          引用的环境已不存在，保存前请重新选择
        </div>
      )}
    </div>
  );
}

// env_config_select 控件：依赖同表单 environment_id；按 kindFilter 过滤
function EnvConfigSelect({
  id,
  value,
  envID,
  kindFilter,
  onChange,
}: {
  id: string;
  value: string;
  envID: string;
  kindFilter?: EnvConfigKind;
  onChange: (v: unknown) => void;
}) {
  const { data: envs, isLoading } = useEnvironments();

  if (!envID) {
    return (
      <div className="rounded border border-slate-200 bg-slate-50 px-2 py-1.5 text-xs text-slate-500">
        请先选择上方的「环境」
      </div>
    );
  }
  if (isLoading) {
    return <div className="text-xs text-slate-400">加载配置列表...</div>;
  }

  const env = envs?.find((e) => e.id === envID);
  if (!env) {
    return (
      <div className="rounded border border-red-200 bg-red-50 px-2 py-1.5 text-xs text-red-700">
        所选环境已不存在
      </div>
    );
  }

  const configs: EnvConfigItem[] = kindFilter
    ? env.configs.filter((c) => c.kind === kindFilter)
    : env.configs;

  const exists = configs.some((c) => c.id === value);
  const isOrphan = value !== '' && !exists;

  if (configs.length === 0) {
    return (
      <div className="rounded border border-amber-300 bg-amber-50 px-2 py-1.5 text-xs text-amber-700">
        该环境下没有 {kindFilter ?? ''} 类型配置，请到环境详情添加
      </div>
    );
  }

  return (
    <div className="space-y-1">
      <select
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={cn(
          'w-full rounded border bg-white px-2 py-1.5 text-xs',
          isOrphan ? 'border-red-400' : 'border-slate-300',
        )}
      >
        <option value="">（请选择配置）</option>
        {isOrphan && <option value={value}>未定义：{value}</option>}
        {configs.map((c) => (
          <option key={c.id} value={c.id}>
            {c.name}
          </option>
        ))}
      </select>
      {isOrphan && (
        <div className="text-[11px] text-red-600">
          引用的配置已不存在，保存前请重新选择
        </div>
      )}
    </div>
  );
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
