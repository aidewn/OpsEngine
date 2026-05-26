// 环境配置的 kind 动态表单
// 复用 features/workflow/ConfigForm.tsx：按 kind 选 FieldSchema
// Phase 1 仅 ssh；Phase 3 增加 docker（mode=over_ssh + sibling ssh 选择）
// k8s/jenkins 仍占位，待 Phase 4-5 上线

import { cn } from '@/lib/cn';
import { ConfigForm } from '@/features/workflow/ConfigForm';
import type { FieldSchema } from '@/types/nodeType';
import type {
  EnvConfigItem,
  EnvConfigKind,
  EnvironmentDef,
} from '@/types/environment';

interface Props {
  kind: EnvConfigKind;
  value: Record<string, unknown>;
  onChange: (next: Record<string, unknown>) => void;
  // docker over_ssh 需要从同环境的 ssh 配置中挑选 ssh_config_id
  // 编辑既有项时需要排除自身（避免循环引用，理论上 docker 不会引用 docker，但保留扩展性）
  environment?: EnvironmentDef;
  editingItemID?: string;
}

// SSH 字段对齐 internal/nodes/ssh_with_linux/node.go:47-54，确保 TestEnvConfig 与节点行为一致
const SSH_SCHEMA: FieldSchema[] = [
  {
    type: 'text',
    id: 'host',
    label: 'IP / Host',
    placeholder: '192.168.1.10',
    required: true,
  },
  {
    type: 'number',
    id: 'port',
    label: 'SSH 端口',
    required: true,
    min: 1,
    max: 65535,
    default: 22,
  },
  {
    type: 'text',
    id: 'user',
    label: '用户名',
    placeholder: 'root',
    required: true,
  },
  {
    type: 'password',
    id: 'password',
    label: '密码',
    required: true,
  },
  {
    type: 'number',
    id: 'timeout_seconds',
    label: '超时（秒）',
    min: 1,
    max: 300,
    default: 10,
  },
];

// K8s schema：kubeconfig YAML + 可选 context / namespace
const K8S_SCHEMA: FieldSchema[] = [
  {
    type: 'textarea',
    id: 'kubeconfig_yaml',
    label: 'kubeconfig YAML',
    placeholder: '粘贴完整 kubeconfig（含 clusters / users / contexts）',
    required: true,
  },
  {
    type: 'text',
    id: 'context',
    label: 'Context 名（可选）',
    placeholder: '留空则使用 kubeconfig 当前上下文',
  },
  {
    type: 'text',
    id: 'namespace',
    label: '默认 Namespace（可选）',
    placeholder: '留空则继承上下文中的 namespace，否则 default',
  },
];

// Jenkins schema：base_url + user + api_token
const JENKINS_SCHEMA: FieldSchema[] = [
  {
    type: 'text',
    id: 'base_url',
    label: 'Base URL',
    placeholder: 'https://jenkins.example.com',
    required: true,
  },
  { type: 'text', id: 'user', label: '用户名', required: true },
  { type: 'password', id: 'api_token', label: 'API Token', required: true },
];

// Localhost schema：无任何字段。fields 落库为空对象，节点直接挂载即可
// 表单展示由 EnvConfigForm 主体内的提示文案承担，避免用户看到空白以为坏掉
const LOCALHOST_SCHEMA: FieldSchema[] = [];

// Docker 通用 schema（mode + socket_path）；ssh_config_id 由专用控件维护，不放进 ConfigForm
const DOCKER_SCHEMA: FieldSchema[] = [
  {
    type: 'select',
    id: 'mode',
    label: '连接模式',
    options: ['over_ssh'],
    default: 'over_ssh',
    required: true,
  },
  {
    type: 'text',
    id: 'socket_path',
    label: 'Docker socket 路径',
    placeholder: '/var/run/docker.sock',
    default: '/var/run/docker.sock',
  },
];

const SCHEMA_BY_KIND: Partial<Record<EnvConfigKind, FieldSchema[]>> = {
  ssh: SSH_SCHEMA,
  docker: DOCKER_SCHEMA,
  k8s: K8S_SCHEMA,
  jenkins: JENKINS_SCHEMA,
  localhost: LOCALHOST_SCHEMA,
};

// 收集 schema 默认值，构造新增配置时的初始 fields
export function defaultFieldsForKind(
  kind: EnvConfigKind,
): Record<string, unknown> {
  const schema = SCHEMA_BY_KIND[kind] ?? [];
  const out: Record<string, unknown> = {};
  for (const f of schema) {
    if (f.default !== undefined && f.default !== null) {
      out[f.id] = f.default;
    }
  }
  return out;
}

export function EnvConfigForm({
  kind,
  value,
  onChange,
  environment,
  editingItemID,
}: Props) {
  // 5 种 kind 全部就位（ssh/docker/k8s/jenkins/localhost）；若联合类型扩展，schema 缺失时给出明确提示
  const schema = SCHEMA_BY_KIND[kind];
  if (!schema) {
    return (
      <div className="rounded border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-700">
        kind「{kind}」暂未实现配置表单
      </div>
    );
  }
  return (
    <div className="space-y-3">
      {kind === 'localhost' && (
        <div className="rounded border border-slate-200 bg-white px-3 py-2 text-xs text-slate-600">
          本机配置无需填写任何字段，保存后即可在工作流中通过「环境 · 本机连接」节点直接挂载使用。
        </div>
      )}
      <ConfigForm schema={schema} value={value} onChange={onChange} />
      {kind === 'docker' && (
        <SiblingConfigSelect
          label="SSH 引用"
          environment={environment}
          editingItemID={editingItemID}
          kindFilter="ssh"
          value={(value['ssh_config_id'] as string) ?? ''}
          onChange={(v) => onChange({ ...value, ssh_config_id: v })}
        />
      )}
    </div>
  );
}

// SiblingConfigSelect 在 EnvironmentDetailPage 内部使用：
// 从当前正在编辑的 environment.configs 中按 kind 过滤出可选项
// 不依赖 useEnvironments 拉取，因为本地表单的修改尚未保存
function SiblingConfigSelect({
  label,
  environment,
  editingItemID,
  kindFilter,
  value,
  onChange,
}: {
  label: string;
  environment?: EnvironmentDef;
  editingItemID?: string;
  kindFilter: EnvConfigKind;
  value: string;
  onChange: (v: string) => void;
}) {
  if (!environment) {
    return (
      <div className="rounded border border-amber-300 bg-amber-50 px-2 py-1.5 text-xs text-amber-700">
        无法读取当前环境（开发者错误：忘记传入 environment prop）
      </div>
    );
  }
  const siblings: EnvConfigItem[] = environment.configs.filter(
    (c) => c.kind === kindFilter && c.id !== editingItemID,
  );
  const exists = siblings.some((c) => c.id === value);
  const isOrphan = value !== '' && !exists;

  if (siblings.length === 0) {
    return (
      <div className="space-y-1">
        <div className="text-xs font-medium text-slate-600">{label}</div>
        <div className="rounded border border-amber-300 bg-amber-50 px-2 py-1.5 text-xs text-amber-700">
          该环境下还没有 {kindFilter} 配置；请先添加 SSH 配置再回来选择
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-1">
      <label className="text-xs font-medium text-slate-600">{label}</label>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={cn(
          'w-full rounded border bg-white px-2 py-1.5 text-xs',
          isOrphan ? 'border-red-400' : 'border-slate-300',
        )}
      >
        <option value="">（请选择 SSH 配置）</option>
        {isOrphan && <option value={value}>未定义：{value}</option>}
        {siblings.map((c) => (
          <option key={c.id} value={c.id}>
            {c.name}
          </option>
        ))}
      </select>
      {isOrphan && (
        <div className="text-[11px] text-red-600">
          引用的 SSH 配置已不存在，保存前请重新选择
        </div>
      )}
    </div>
  );
}
