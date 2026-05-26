// 环境详情页：编辑名称/描述，管理 configs 列表
// 表单型（非画布），名称/描述本地状态 + 500ms debounce 自动保存

import { useEffect, useRef, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { Button } from '@/components/ui/Button';
import { Dialog } from '@/components/ui/Dialog';
import { Input } from '@/components/ui/Input';
import { Label } from '@/components/ui/Label';
import { Textarea } from '@/components/ui/Textarea';
import { CenteredMessage } from '@/components/ui/CenteredMessage';
import {
  useEnvironment,
  useUpdateEnvironment,
  useTestEnvConfig,
} from '@/api/environments';
import type { EnvConfigItem, EnvironmentDef } from '@/types/environment';
import { EnvConfigDialog } from '@/features/environment/EnvConfigDialog';

// 测试连接结果，按 configID 维度展示
interface TestResultState {
  configID: string;
  ok: boolean;
  message: string;
}

export function EnvironmentDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: env, isLoading, error } = useEnvironment(id);
  const update = useUpdateEnvironment();
  const test = useTestEnvConfig();

  // 名称/描述：本地受控 + debounce 自动保存
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  // 上一次从服务端拿到的值，作为 debounce flush 是否需要触发的基准
  const baseline = useRef<{ name: string; description: string } | null>(null);

  // 服务端数据加载完成后初始化本地状态
  useEffect(() => {
    if (!env) return;
    setName(env.name);
    setDescription(env.description);
    baseline.current = { name: env.name, description: env.description };
  }, [env?.id]);

  // 500ms debounce 自动保存名称/描述
  useEffect(() => {
    if (!env || !baseline.current) return;
    const trimmedName = name.trim();
    const trimmedDesc = description.trim();
    const base = baseline.current;
    if (trimmedName === base.name && trimmedDesc === base.description) return;
    if (trimmedName === '') return; // 空名称不入库，由后端 validate 也会拒，这里短路避免无意义请求
    const timer = setTimeout(() => {
      update.mutate(
        { ...env, name: trimmedName, description: trimmedDesc },
        {
          onSuccess: () => {
            baseline.current = { name: trimmedName, description: trimmedDesc };
          },
        },
      );
    }, 500);
    return () => clearTimeout(timer);
    // 仅依赖输入值与 env.id；其它字段变化（configs）走自己的 Dialog 路径
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [name, description, env?.id]);

  // 配置项 Dialog 状态
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editItem, setEditItem] = useState<EnvConfigItem | null>(null);

  // 删除配置项的二次确认
  const [deleteConfig, setDeleteConfig] = useState<EnvConfigItem | null>(null);

  // 测试连接结果（单一状态：始终展示最近一次结果）
  const [testResult, setTestResult] = useState<TestResultState | null>(null);

  if (isLoading) {
    return <CenteredMessage>加载中...</CenteredMessage>;
  }
  if (error) {
    return <CenteredMessage tone="error">加载失败：{error.message}</CenteredMessage>;
  }
  if (!env) {
    return <CenteredMessage tone="error">环境不存在</CenteredMessage>;
  }

  async function handleTest(item: EnvConfigItem) {
    setTestResult(null);
    try {
      await test.mutateAsync({ envID: env!.id, configID: item.id });
      setTestResult({
        configID: item.id,
        ok: true,
        message: `${item.name} 连接成功`,
      });
    } catch (err) {
      setTestResult({
        configID: item.id,
        ok: false,
        message: err instanceof Error ? err.message : '连接失败',
      });
    }
  }

  async function handleDeleteConfig(item: EnvConfigItem) {
    if (!env) return;
    const next: EnvironmentDef = {
      ...env,
      configs: env.configs.filter((c) => c.id !== item.id),
    };
    try {
      await update.mutateAsync(next);
      setDeleteConfig(null);
      if (testResult?.configID === item.id) setTestResult(null);
    } catch (err) {
      console.error('删除配置失败:', err);
    }
  }

  return (
    <div className="mx-auto flex h-full max-w-3xl flex-col px-6 py-8">
      <header className="mb-4">
        <Link
          to="/"
          className="text-xs text-slate-500 hover:text-slate-700"
        >
          ← 返回首页
        </Link>
        <h1 className="mt-2 text-2xl font-semibold text-slate-900">环境配置</h1>
        <div className="mt-0.5 font-mono text-xs text-slate-500">{env.id}</div>
      </header>

      <section className="mb-8 space-y-4 rounded-lg border border-slate-200 bg-white p-4">
        <div className="space-y-1">
          <Label htmlFor="env-name">名称</Label>
          <Input
            id="env-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="env-desc">描述</Label>
          <Textarea
            id="env-desc"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={2}
          />
        </div>
      </section>

      <section>
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-slate-900">配置列表</h2>
          <Button
            size="sm"
            onClick={() => {
              setEditItem(null);
              setDialogOpen(true);
            }}
          >
            + 添加配置
          </Button>
        </div>

        {testResult && (
          <div
            className={
              testResult.ok
                ? 'mb-3 rounded border border-green-200 bg-green-50 px-3 py-2 text-xs text-green-700'
                : 'mb-3 rounded border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700'
            }
          >
            {testResult.message}
          </div>
        )}

        {env.configs.length === 0 ? (
          <div className="rounded-lg border border-dashed border-slate-300 bg-white px-6 py-12 text-center text-sm text-slate-500">
            还没有配置，点击右上角添加
          </div>
        ) : (
          <ul className="divide-y divide-slate-200 rounded-lg border border-slate-200 bg-white">
            {env.configs.map((c) => (
              <li
                key={c.id}
                className="flex items-center justify-between gap-3 px-4 py-3"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-slate-900">
                      {c.name}
                    </span>
                    <span className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] font-medium uppercase text-slate-600">
                      {c.kind}
                    </span>
                  </div>
                  {c.description && (
                    <div className="mt-0.5 text-xs text-slate-500">
                      {c.description}
                    </div>
                  )}
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => handleTest(c)}
                    disabled={test.isPending}
                  >
                    {test.isPending && test.variables?.configID === c.id
                      ? '测试中...'
                      : '测试连接'}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      setEditItem(c);
                      setDialogOpen(true);
                    }}
                  >
                    编辑
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setDeleteConfig(c)}
                  >
                    删除
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>

      <EnvConfigDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        environment={env}
        editItem={editItem}
      />

      {deleteConfig && (
        <Dialog
          open={true}
          onOpenChange={(open) => {
            if (!open) setDeleteConfig(null);
          }}
          title="确认删除配置"
          description={`确定要从环境中删除「${deleteConfig.name}」吗？`}
          footer={
            <>
              <Button
                type="button"
                variant="secondary"
                onClick={() => setDeleteConfig(null)}
                disabled={update.isPending}
              >
                取消
              </Button>
              <Button
                type="button"
                variant="danger"
                onClick={() => handleDeleteConfig(deleteConfig)}
                disabled={update.isPending}
              >
                {update.isPending ? '删除中...' : '确认删除'}
              </Button>
            </>
          }
        >
          <div className="text-sm text-slate-600">
            配置 ID: <span className="font-mono">{deleteConfig.id}</span>
          </div>
        </Dialog>
      )}
    </div>
  );
}
