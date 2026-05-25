// parallel 节点的 branch_count 改小后，清理超范围的 exec_out_<i> 边
// 由 page 层在 handleGraphChange 时调用

import type { GraphDef } from './canvasMapping';

const PARALLEL_DEFAULT = 4;

// effectiveBranchCount 从 parallel 节点 config 解析有效分支数（限制 2-8）
export function effectiveBranchCount(config: Record<string, unknown>): number {
  const raw = Number(config.branch_count);
  if (!Number.isFinite(raw) || raw <= 0) return PARALLEL_DEFAULT;
  return Math.max(2, Math.min(8, Math.floor(raw)));
}

// cleanupParallelEdges 删除所有 parallel 节点的超范围 exec_out_<i> 边
// 返回新的 graph；如果没有需要清理的边，返回原 graph
export function cleanupParallelEdges<T extends GraphDef>(graph: T): T {
  // 找出所有 parallel 节点 + 其有效分支数
  const parallels = new Map<string, number>(); // instance_id → 有效分支数
  for (const n of graph.nodes) {
    if (n.type_id === 'parallel') {
      parallels.set(n.instance_id, effectiveBranchCount(n.config));
    }
  }
  if (parallels.size === 0) return graph;

  const filtered = graph.edges.filter((e) => {
    const limit = parallels.get(e.from.node);
    if (limit === undefined) return true;
    const m = /^exec_out_(\d+)$/.exec(e.from.port);
    if (!m) return true; // exec_out_done / 其他端口保留
    const idx = parseInt(m[1] ?? '0', 10);
    return idx <= limit;
  });

  if (filtered.length === graph.edges.length) return graph;
  return { ...graph, edges: filtered };
}
