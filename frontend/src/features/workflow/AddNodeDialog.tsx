// 添加节点弹窗
// 两种触发方式：
//   1. 顶部「+ 添加节点」按钮 → 创建在画布中心
//   2. 拖连线到空白处 → 只显示兼容端口的节点类型，创建在松手位置并自动连线

import { useMemo, useState } from 'react';
import { Dialog } from '@/components/ui/Dialog';
import { Input } from '@/components/ui/Input';
import { useNodeTypes } from '@/api/nodeTypes';
import type { NodeTypeDef } from '@/types/nodeType';
import { isSystemNodeType, resolvePortType } from '@/types/nodeType';
import type { PendingConnection } from './WorkflowCanvas';

interface AddNodeDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // 拖线上下文（为 null 表示从按钮打开，不过滤）
  pendingConnection: PendingConnection | null;
  // 选中节点类型后的回调
  onSelect: (typeDef: NodeTypeDef, matchedPortId: string | null) => void;
}

export function AddNodeDialog({
  open,
  onOpenChange,
  pendingConnection,
  onSelect,
}: AddNodeDialogProps) {
  const { data: nodeTypes } = useNodeTypes();
  const [search, setSearch] = useState('');

  // 过滤逻辑：按搜索词 + 拖线端口兼容性
  const filtered = useMemo(() => {
    if (!nodeTypes) return [];

    let types = nodeTypes.filter((t) => !isSystemNodeType(t.type_id));

    // 搜索过滤
    if (search.trim()) {
      const q = search.trim().toLowerCase();
      types = types.filter(
        (t) =>
          t.display_name.toLowerCase().includes(q) ||
          t.category.toLowerCase().includes(q) ||
          t.type_id.toLowerCase().includes(q),
      );
    }

    // 拖线兼容性过滤
    if (pendingConnection) {
      types = types.filter((t) =>
        findCompatiblePort(t, pendingConnection) !== null,
      );
    }

    return types;
  }, [nodeTypes, search, pendingConnection]);

  // 按 category 分组
  const grouped = useMemo(() => {
    const map = new Map<string, NodeTypeDef[]>();
    for (const t of filtered) {
      const group = map.get(t.category) ?? [];
      group.push(t);
      map.set(t.category, group);
    }
    return map;
  }, [filtered]);

  function handleSelect(typeDef: NodeTypeDef) {
    const matchedPortId = pendingConnection
      ? findCompatiblePort(typeDef, pendingConnection)
      : null;
    onSelect(typeDef, matchedPortId);
    setSearch('');
    onOpenChange(false);
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) setSearch('');
        onOpenChange(v);
      }}
      title="添加节点"
      description={
        pendingConnection
          ? `选择一个有 ${pendingConnection.sourcePortType} 端口的节点`
          : '选择要添加的节点类型'
      }
    >
      <div className="space-y-3">
        <Input
          placeholder="搜索节点类型..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          autoFocus
        />

        <div className="max-h-64 overflow-y-auto">
          {grouped.size === 0 && (
            <div className="py-6 text-center text-sm text-slate-400">
              {nodeTypes?.length === 0
                ? '暂无已注册的节点类型'
                : '没有匹配的节点类型'}
            </div>
          )}

          {[...grouped.entries()].map(([category, types]) => (
            <div key={category} className="mb-2">
              <div className="sticky top-0 bg-white px-1 py-1 text-[10px] font-semibold uppercase tracking-wider text-slate-400">
                {category}
              </div>
              {types.map((t) => (
                <button
                  key={t.type_id}
                  type="button"
                  onClick={() => handleSelect(t)}
                  className="flex w-full items-center gap-3 rounded px-2 py-2 text-left text-sm hover:bg-slate-50"
                >
                  <span className="text-lg">{t.icon || '◆'}</span>
                  <div className="min-w-0 flex-1">
                    <div className="font-medium text-slate-900">
                      {t.display_name}
                    </div>
                    {t.description && (
                      <div className="truncate text-xs text-slate-500">
                        {t.description}
                      </div>
                    )}
                  </div>
                  <span className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-500">
                    {t.node_kind}
                  </span>
                </button>
              ))}
            </div>
          ))}
        </div>
      </div>
    </Dialog>
  );
}

// 在目标节点类型中查找与 pendingConnection 端口兼容的端口
// 返回匹配的端口 ID，无匹配返回 null
function findCompatiblePort(
  typeDef: NodeTypeDef,
  pending: PendingConnection,
): string | null {
  // 从 output 拖出 → 需要目标的 input 端口匹配
  // 从 input 拖出 → 需要目标的 output 端口匹配
  const candidatePorts =
    pending.sourceDirection === 'output'
      ? typeDef.input_ports
      : typeDef.output_ports;

  for (const port of candidatePorts) {
    // 对候选端口也需要解析 Dynamic 类型，但新节点 config 为空
    // Dynamic 未配置时默认 String，所以只有 pending 也是 String 才匹配
    const portType = resolvePortType(port, {});
    if (portType === pending.sourcePortType) {
      return port.id;
    }
  }
  return null;
}
