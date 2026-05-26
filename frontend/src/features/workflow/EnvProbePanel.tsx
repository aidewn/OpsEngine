// 探测节点面板：嵌入 NodeDetailPanel，置于 ConfigForm 之下
//
// 用户路径：
//   1. 填好 environment_id / config_id / path 等
//   2. 点击「探测一次」→ ProbeEnvNode → 列出可选项
//   3. 点选某项 → 高亮
//   4. 点击「应用」→ 写 node.config.probe_snapshot；可选勾选「同步到工作流变量」
//      根据 variable_bindings 更新 workflow.variables[].default
//
// 仅在节点 TypeID 以 env_probe_ 开头时渲染（由 NodeDetailPanel 决定挂载与否）

import { useMemo, useState } from 'react';
import { Button } from '@/components/ui/Button';
import { useProbeEnvNode } from '@/api/probe';
import type {
  ProbeItem,
  ProbeSnapshot,
  VariableBinding,
} from '@/types/probe';
import type { NodeInstance, VariableDef } from '@/types/workflow';
import type { NodeTypeDef } from '@/types/nodeType';
import { cn } from '@/lib/cn';

interface Props {
  node: NodeInstance;
  nodeType: NodeTypeDef;
  variables: VariableDef[];
  // 写 config（与 ConfigForm 的 onChange 同源）
  onConfigChange: (config: Record<string, unknown>) => void;
  // 可选：同步快照到 workflow.variables[].default 时使用；不传则隐藏「同步到变量」开关
  onVariablesChange?: (next: VariableDef[]) => void;
}

export function EnvProbePanel({
  node,
  nodeType,
  variables,
  onConfigChange,
  onVariablesChange,
}: Props) {
  const probe = useProbeEnvNode();

  // 现有快照（已应用的）从 config 读出
  const existingSnapshot = node.config['probe_snapshot'] as
    | ProbeSnapshot
    | undefined;

  // 现有 variable_bindings 从 config 读出（不存在则空数组）
  const existingBindings = useMemo<VariableBinding[]>(() => {
    const raw = node.config['variable_bindings'];
    return Array.isArray(raw) ? (raw as VariableBinding[]) : [];
  }, [node.config]);

  // 本次探测后的结果（未应用前）
  const [items, setItems] = useState<ProbeItem[] | null>(null);
  // 本次选中项的 key
  const [pickedKey, setPickedKey] = useState<string>(
    existingSnapshot?.picked_key ?? '',
  );
  // 同步变量开关 + 当前编辑的 bindings
  const [syncVars, setSyncVars] = useState<boolean>(
    existingBindings.length > 0,
  );
  const [bindings, setBindings] = useState<VariableBinding[]>(existingBindings);

  // 节点的 output ports（用作 binding 的可选 output_port）
  const outputPorts = (nodeType.output_ports ?? []).filter(
    (p) => p.port_type !== 'Exec',
  );

  const envID = (node.config['environment_id'] as string) ?? '';
  const configID = (node.config['config_id'] as string) ?? '';
  const canProbe = !!envID && !!configID && !probe.isPending;

  // 当前节点的 resolve_mode；static 模式下没有快照会直接 silently 返回空，提示用户必须探测一次
  const resolveMode = (node.config['resolve_mode'] as string) || 'static';
  const missingSnapshot = !existingSnapshot && resolveMode === 'static';

  // 快照「陈旧」阈值：7 天；超过给黄色提示，仍可用
  const snapshotStaleMs = 7 * 24 * 60 * 60 * 1000;
  const snapshotAgeMs = existingSnapshot
    ? Date.now() - new Date(existingSnapshot.captured_at).getTime()
    : 0;
  const isSnapshotStale =
    !!existingSnapshot && snapshotAgeMs > snapshotStaleMs;

  async function handleProbe() {
    setItems(null);
    try {
      const res = await probe.mutateAsync({
        type_id: node.type_id,
        env_id: envID,
        config_id: configID,
        node_config: node.config,
      });
      setItems(res.items);
      // 探测后不强制清空已选；若新结果不含旧选项则用户需重新选
      if (res.items.findIndex((it) => it.key === pickedKey) < 0) {
        setPickedKey(res.items[0]?.key ?? '');
      }
    } catch (err) {
      setItems([]);
      console.error('探测失败:', err);
    }
  }

  function handleApply() {
    // 使用最近的探测结果；如果未探测过但有快照，沿用快照 items
    const sourceItems = items ?? existingSnapshot?.items ?? [];
    const picked = sourceItems.find((it) => it.key === pickedKey);
    if (!picked) return;

    const snapshot: ProbeSnapshot = {
      picked_key: picked.key,
      picked_label: picked.label,
      items: sourceItems,
      captured_at: new Date().toISOString(),
    };

    const nextConfig: Record<string, unknown> = {
      ...node.config,
      probe_snapshot: snapshot,
    };
    // 同步开关关闭时显式清掉 bindings，避免残留
    nextConfig.variable_bindings = syncVars ? bindings : [];
    onConfigChange(nextConfig);

    // 同步到 workflow.variables[].default
    if (syncVars && onVariablesChange && variables.length > 0) {
      const next = variables.map((v) => {
        const b = bindings.find((x) => x.variable_name === v.name);
        if (!b) return v;
        const port = outputPorts.find((p) => p.id === b.output_port);
        if (!port) return v;
        let val: unknown = '';
        if (b.output_port === 'selected_path') val = picked.key;
        else if (b.output_port === 'paths')
          val = sourceItems.map((it) => it.key);
        else if (b.output_port === 'count') val = sourceItems.length;
        return { ...v, default: val };
      });
      onVariablesChange(next);
    }
  }

  function updateBinding(idx: number, patch: Partial<VariableBinding>) {
    setBindings((prev) =>
      prev.map((b, i) => (i === idx ? { ...b, ...patch } : b)),
    );
  }
  function addBinding() {
    setBindings((prev) => [...prev, { variable_name: '', output_port: '' }]);
  }
  function removeBinding(idx: number) {
    setBindings((prev) => prev.filter((_, i) => i !== idx));
  }

  return (
    <section className="space-y-3 border-t border-slate-200 pt-3">
      <header className="flex items-center justify-between">
        <h3 className="text-xs font-semibold text-slate-700">编辑态探测</h3>
        <Button size="sm" onClick={handleProbe} disabled={!canProbe}>
          {probe.isPending ? '探测中...' : '探测一次'}
        </Button>
      </header>

      {!envID || !configID ? (
        <div className="text-xs text-slate-500">
          请先在上方 Config 选择环境与 SSH 配置
        </div>
      ) : null}

      {probe.error && (
        <div className="rounded border border-red-200 bg-red-50 px-2 py-1.5 text-xs text-red-700">
          探测失败：{probe.error.message}
        </div>
      )}

      {/* static 模式 + 无快照：明确警告，避免运行时下游拿到空值 */}
      {missingSnapshot && (
        <div className="rounded border border-red-200 bg-red-50 px-2 py-1.5 text-xs text-red-700">
          static 模式下尚未应用快照，运行时下游会拿到空值。请「探测一次」并「应用」。
        </div>
      )}

      {existingSnapshot && !items && (
        <div
          className={cn(
            'rounded border px-2 py-1.5 text-xs',
            isSnapshotStale
              ? 'border-amber-300 bg-amber-50 text-amber-700'
              : 'border-slate-200 bg-slate-50 text-slate-600',
          )}
        >
          已应用快照：
          <span className="font-mono">{existingSnapshot.picked_label}</span>
          <span className="ml-2 text-[11px] text-slate-400">
            {new Date(existingSnapshot.captured_at).toLocaleString()}
          </span>
          {isSnapshotStale && (
            <span className="ml-2 text-[11px] text-amber-700">
              · 已超过 {Math.floor(snapshotAgeMs / (24 * 60 * 60 * 1000))} 天，建议重新探测
            </span>
          )}
        </div>
      )}

      {items !== null && (
        <div className="space-y-1">
          {items.length === 0 ? (
            <div className="text-xs text-slate-400">（无结果）</div>
          ) : (
            <ul className="max-h-48 overflow-y-auto rounded border border-slate-200 bg-white">
              {items.map((it) => (
                <li key={it.key}>
                  <button
                    type="button"
                    onClick={() => setPickedKey(it.key)}
                    className={cn(
                      'w-full px-2 py-1.5 text-left text-xs hover:bg-slate-50',
                      pickedKey === it.key && 'bg-blue-50 text-blue-700',
                    )}
                  >
                    <div className="truncate font-mono">{it.label}</div>
                    <div className="truncate text-[11px] text-slate-400">
                      {it.key}
                    </div>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      {/* variable_bindings 编辑 */}
      {onVariablesChange && (
        <div className="space-y-1 rounded border border-slate-200 bg-slate-50 px-2 py-2">
          <label className="flex items-center gap-2 text-xs">
            <input
              type="checkbox"
              checked={syncVars}
              onChange={(e) => setSyncVars(e.target.checked)}
              className="size-3.5 rounded border-slate-300 accent-blue-600"
            />
            同步快照到工作流变量
          </label>
          {syncVars && (
            <div className="space-y-1">
              {bindings.length === 0 && (
                <div className="text-[11px] text-slate-400">
                  尚未添加 binding
                </div>
              )}
              {bindings.map((b, idx) => (
                <div key={idx} className="flex items-center gap-1">
                  <select
                    value={b.variable_name}
                    onChange={(e) =>
                      updateBinding(idx, { variable_name: e.target.value })
                    }
                    className="flex-1 rounded border border-slate-300 bg-white px-1.5 py-1 text-[11px]"
                  >
                    <option value="">（变量）</option>
                    {variables.map((v) => (
                      <option key={v.name} value={v.name}>
                        {v.name}
                      </option>
                    ))}
                  </select>
                  <span className="text-[11px] text-slate-400">←</span>
                  <select
                    value={b.output_port}
                    onChange={(e) =>
                      updateBinding(idx, { output_port: e.target.value })
                    }
                    className="flex-1 rounded border border-slate-300 bg-white px-1.5 py-1 text-[11px]"
                  >
                    <option value="">（端口）</option>
                    {outputPorts.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.id}
                      </option>
                    ))}
                  </select>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => removeBinding(idx)}
                  >
                    ✕
                  </Button>
                </div>
              ))}
              <Button variant="secondary" size="sm" onClick={addBinding}>
                + 添加 binding
              </Button>
              {variables.length === 0 && (
                <div className="text-[11px] text-amber-700">
                  当前图无变量；请先在左侧变量面板添加
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {(items !== null || existingSnapshot) && (
        <Button
          size="sm"
          onClick={handleApply}
          disabled={
            !pickedKey ||
            (items !== null && items.length === 0) ||
            (syncVars &&
              bindings.some((b) => !b.variable_name || !b.output_port))
          }
        >
          应用
        </Button>
      )}
    </section>
  );
}
