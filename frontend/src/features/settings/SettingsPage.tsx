// 设置页，当前用于维护外部 AI 模型连接参数。

import { FormEvent, type ReactNode, useEffect, useState } from 'react';
import { Button } from '@/components/ui/Button';
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
};

export function SettingsPage() {
  const { data, isLoading, error } = useAISettings();
  const updateSettings = useUpdateAISettings();
  const testSettings = useTestAISettings();
  const [form, setForm] = useState<AISettings>(DEFAULT_SETTINGS);
  const [message, setMessage] = useState('');

  useEffect(() => {
    if (data) {
      setForm({
        deepseek_api_key: data.deepseek_api_key ?? '',
        deepseek_base_url:
          data.deepseek_base_url || DEFAULT_SETTINGS.deepseek_base_url,
        deepseek_model: data.deepseek_model || DEFAULT_SETTINGS.deepseek_model,
        timeout_seconds:
          data.timeout_seconds || DEFAULT_SETTINGS.timeout_seconds,
      });
    }
  }, [data]);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setMessage('');
    try {
      await updateSettings.mutateAsync(form);
      setMessage('AI 设置已保存');
    } catch (err) {
      setMessage(err instanceof Error ? err.message : '保存失败');
    }
  }

  async function handleTest() {
    setMessage('');
    try {
      await updateSettings.mutateAsync(form);
      const result = await testSettings.mutateAsync();
      setMessage(`连接测试返回：${result}`);
    } catch (err) {
      setMessage(err instanceof Error ? err.message : '连接测试失败');
    }
  }

  return (
    <section className="max-w-2xl">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold text-slate-900">设置</h1>
        <p className="mt-1 text-sm text-slate-500">
          配置 DeepSeek API，用于 AI 对话和后续工作流生成。
        </p>
      </header>

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
          <input
            type="password"
            value={form.deepseek_api_key}
            onChange={(event) =>
              setForm((prev) => ({
                ...prev,
                deepseek_api_key: event.target.value,
              }))
            }
            placeholder="sk-..."
            className="h-9 w-full rounded-md border border-slate-300 px-3 text-sm outline-none focus:border-slate-500"
          />
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
            className="h-9 w-full rounded-md border border-slate-300 px-3 text-sm outline-none focus:border-slate-500"
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
            className="h-9 w-full rounded-md border border-slate-300 px-3 text-sm outline-none focus:border-slate-500"
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
            className="h-9 w-32 rounded-md border border-slate-300 px-3 text-sm outline-none focus:border-slate-500"
          />
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
