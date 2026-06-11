// 设置页，当前用于维护外部 AI 模型连接参数。

import { FormEvent, type ReactNode, useEffect, useState } from 'react';
import { Button } from '@/components/ui/Button';
import { Select } from '@/components/ui/Select';
import {
  useAISettings,
  useTestAISettings,
  useUpdateAISettings,
} from '@/api/ai';
import type { AISettings } from '@/types/ai';

// DEFAULT_SETTINGS 是前端表单初始值。
const DEFAULT_SETTINGS: AISettings = {
  deepseek_api_key: '',
  deepseek_base_url: 'https://api.deepseek.com',
  deepseek_model: 'deepseek-chat',
  timeout_seconds: 60,
  apply_mode: 'confirm',
};

interface SettingsPageProps {
  embedded?: boolean;
}

export function SettingsPage({ embedded = false }: SettingsPageProps) {
  const { data, isLoading, error } = useAISettings();
  const updateSettings = useUpdateAISettings();
  const testSettings = useTestAISettings();
  const [form, setForm] = useState<AISettings>(DEFAULT_SETTINGS);
  const [message, setMessage] = useState('');
  // editingKey=true 时让用户输入 API Key；false 时显示脱敏的旧值 + 编辑按钮。
  // 避免每次进入设置页都把 Key 明文回填到 input。
  const [editingKey, setEditingKey] = useState(false);
  const [revealKey, setRevealKey] = useState(false);

  useEffect(() => {
    if (data) {
      setForm({
        deepseek_api_key: data.deepseek_api_key ?? '',
        deepseek_base_url:
          data.deepseek_base_url || DEFAULT_SETTINGS.deepseek_base_url,
        deepseek_model: data.deepseek_model || DEFAULT_SETTINGS.deepseek_model,
        timeout_seconds:
          data.timeout_seconds || DEFAULT_SETTINGS.timeout_seconds,
        apply_mode: data.apply_mode === 'auto' ? 'auto' : 'confirm',
      });
      // 已有 Key 时默认进入"已保存"态，避免明文回填；空 Key 必须进入编辑态让用户填。
      setEditingKey(!data.deepseek_api_key);
      setRevealKey(false);
    }
  }, [data]);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setMessage('');
    try {
      await updateSettings.mutateAsync(form);
      setMessage('AI 设置已保存');
      setEditingKey(false);
      setRevealKey(false);
    } catch (err) {
      setMessage(formatLLMError(err));
    }
  }

  async function handleTest() {
    setMessage('');
    try {
      await updateSettings.mutateAsync(form);
      const result = await testSettings.mutateAsync();
      setMessage(`连接测试返回：${result}`);
      setEditingKey(false);
      setRevealKey(false);
    } catch (err) {
      setMessage(formatLLMError(err));
    }
  }

  return (
    <section className="max-w-2xl">
      {!embedded ? (
        <header className="mb-6">
          <h1 className="text-2xl font-semibold text-slate-900">设置</h1>
          <p className="mt-1 text-sm text-slate-500">
            配置 DeepSeek API，用于 AI 对话和后续工作流生成。
          </p>
        </header>
      ) : null}

      {isLoading && <div className="text-sm text-slate-500">加载中...</div>}
      {error && (
        <div className="mb-4 rounded border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          加载设置失败：{error.message}
        </div>
      )}

      <form
        onSubmit={handleSubmit}
        className="space-y-5 rounded-lg border border-slate-200 bg-white p-5"
      >
        <Field label="DeepSeek API Key">
          {editingKey ? (
            <div className="flex items-stretch gap-2">
              <input
                type={revealKey ? 'text' : 'password'}
                value={form.deepseek_api_key}
                onChange={(event) =>
                  setForm((prev) => ({
                    ...prev,
                    deepseek_api_key: event.target.value,
                  }))
                }
                placeholder="sk-..."
                className="h-9 w-full rounded-md border border-slate-300 px-3 text-sm outline-none focus:border-ops-border-focus font-mono"
                autoFocus
              />
              <button
                type="button"
                onClick={() => setRevealKey((v) => !v)}
                className="rounded-md border border-slate-300 px-2 text-xs text-slate-600 hover:bg-ops-elevated"
                title={revealKey ? '隐藏' : '显示明文'}
              >
                {revealKey ? '隐藏' : '显示'}
              </button>
              {data?.deepseek_api_key && (
                <button
                  type="button"
                  onClick={() => {
                    setEditingKey(false);
                    setRevealKey(false);
                    setForm((prev) => ({
                      ...prev,
                      deepseek_api_key: data.deepseek_api_key,
                    }));
                  }}
                  className="rounded-md border border-slate-300 px-2 text-xs text-slate-600 hover:bg-ops-elevated"
                >
                  取消
                </button>
              )}
            </div>
          ) : (
            <div className="flex items-center gap-2">
              <code className="h-9 flex-1 rounded-md border border-slate-200 bg-slate-50 px-3 text-sm leading-9 text-slate-700 font-mono">
                {maskApiKey(form.deepseek_api_key)}
              </code>
              <button
                type="button"
                onClick={() => setEditingKey(true)}
                className="rounded-md border border-slate-300 px-3 text-xs text-slate-700 hover:bg-ops-elevated"
              >
                编辑
              </button>
            </div>
          )}
        </Field>

        <Field label="Base URL">
          <input
            type="text"
            value={form.deepseek_base_url}
            onChange={(event) =>
              setForm((prev) => ({
                ...prev,
                deepseek_base_url: event.target.value,
              }))
            }
            className="h-9 w-full rounded-md border border-slate-300 px-3 text-sm outline-none focus:border-ops-border-focus"
          />
        </Field>

        <Field label="模型">
          <input
            type="text"
            value={form.deepseek_model}
            onChange={(event) =>
              setForm((prev) => ({
                ...prev,
                deepseek_model: event.target.value,
              }))
            }
            className="h-9 w-full rounded-md border border-slate-300 px-3 text-sm outline-none focus:border-ops-border-focus"
          />
        </Field>

        <Field label="超时时间（秒）">
          <input
            type="number"
            min={1}
            max={300}
            value={form.timeout_seconds}
            onChange={(event) =>
              setForm((prev) => ({
                ...prev,
                timeout_seconds: Number(event.target.value),
              }))
            }
            className="h-9 w-32 rounded-md border border-slate-300 px-3 text-sm outline-none focus:border-ops-border-focus"
          />
        </Field>

        <Field label="AI 修改落盘策略">
          <Select
            className="w-64"
            value={form.apply_mode}
            onChange={(event) =>
              setForm((prev) => ({
                ...prev,
                apply_mode: event.target.value === 'auto' ? 'auto' : 'confirm',
              }))
            }
          >
            <option value="confirm">先确认（生成草案，对话中点应用）</option>
            <option value="auto">直接保存（保存前仍会自动快照）</option>
          </Select>
        </Field>

        {message && (
          <div className="rounded border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-700">
            {message}
          </div>
        )}

        <div className="flex justify-end gap-2">
          <Button
            type="button"
            variant="secondary"
            onClick={handleTest}
            disabled={updateSettings.isPending || testSettings.isPending}
          >
            {updateSettings.isPending || testSettings.isPending
              ? '测试中...'
              : '测试连接'}
          </Button>
          <Button type="submit" disabled={updateSettings.isPending}>
            {updateSettings.isPending ? '保存中...' : '保存设置'}
          </Button>
        </div>
      </form>
    </section>
  );
}

// Field 是设置表单的通用字段布局。
function Field({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-sm font-medium text-slate-700">
        {label}
      </span>
      {children}
    </label>
  );
}

// maskApiKey 把已保存的 API Key 渲染成 "sk-•••cdef"，保留首末 4 字符方便用户对照。
// 空字符串显示占位提示。
function maskApiKey(key: string): string {
  const trimmed = (key ?? '').trim();
  if (!trimmed) return '（未配置）';
  if (trimmed.length <= 8) return '•'.repeat(trimmed.length);
  return `${trimmed.slice(0, 4)}•••${trimmed.slice(-4)}`;
}

// formatLLMError 把后端 LLMError 字符串（形如 "[网络错误] ..."）转成更友好的提示。
// 已经带前缀的直接展示；未带前缀的回退到原始 message。
function formatLLMError(err: unknown): string {
  const msg = err instanceof Error ? err.message : String(err);
  // 后端已经把 LLMError.Kind 拼到前缀里，前端直接展示即可。
  // 此处只做兜底：若没有前缀，附加"请求失败"提示。
  if (/^\[(配置错误|网络错误|模型响应错误)\]/.test(msg)) return msg;
  return msg || '请求失败';
}
