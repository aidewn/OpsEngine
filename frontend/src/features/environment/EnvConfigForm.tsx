// 环境配置的 kind 动态表单
// 复用 features/workflow/ConfigForm.tsx：按 kind 选 FieldSchema；MVP 仅 SSH 完整实现，其它 kind 占位

import { ConfigForm } from '@/features/workflow/ConfigForm';
import type { FieldSchema } from '@/types/nodeType';
import type { EnvConfigKind } from '@/types/environment';

interface Props {
  kind: EnvConfigKind;
  value: Record<string, unknown>;
  onChange: (next: Record<string, unknown>) => void;
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

const SCHEMA_BY_KIND: Partial<Record<EnvConfigKind, FieldSchema[]>> = {
  ssh: SSH_SCHEMA,
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

export function EnvConfigForm({ kind, value, onChange }: Props) {
  const schema = SCHEMA_BY_KIND[kind];
  if (!schema) {
    return (
      <div className="rounded border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-700">
        kind「{kind}」暂未实现配置表单，将在 Phase 2-5 上线
      </div>
    );
  }
  return <ConfigForm schema={schema} value={value} onChange={onChange} />;
}
