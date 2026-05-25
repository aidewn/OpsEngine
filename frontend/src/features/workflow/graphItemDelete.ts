// 删除 variables / params / returns 时，同步清理画布上引用该名称的 getter/setter 节点
// 避免保存校验失败（validateVariableRefs 等）导致乐观更新被回滚

import type { ParamDef } from '@/types/assemble';
import type { NodeInstance, VariableDef } from '@/types/workflow';
import type { GraphDef } from './canvasMapping';

/** 对比前后列表，找出被删除条目的 name */
function removedNames(
  before: { name: string }[],
  after: { name: string }[],
): Set<string> {
  const keep = new Set(after.map((i) => i.name));
  return new Set(before.filter((i) => !keep.has(i.name)).map((i) => i.name));
}

function configName(node: NodeInstance, key: string): string {
  return String(node.config[key] ?? '').trim();
}

/** 移除节点后清理悬空连线 */
function withoutNodes(graph: GraphDef, removedNodeIds: Set<string>): GraphDef {
  if (removedNodeIds.size === 0) return graph;
  return {
    ...graph,
    nodes: graph.nodes.filter((n) => !removedNodeIds.has(n.instance_id)),
    edges: graph.edges.filter(
      (e) =>
        !removedNodeIds.has(e.from.node) && !removedNodeIds.has(e.to.node),
    ),
  };
}

/** 应用 variables 变更；若有删除则移除对应 var_get / var_set 节点 */
export function applyVariablesChange<T extends GraphDef & { variables?: VariableDef[] }>(
  prev: T,
  items: VariableDef[],
): T {
  const removed = removedNames(prev.variables ?? [], items);
  if (removed.size === 0) return { ...prev, variables: items };
  const dropIds = new Set(
    prev.nodes
      .filter((n) => {
        if (n.type_id !== 'var_get' && n.type_id !== 'var_set') return false;
        return removed.has(configName(n, 'var_name'));
      })
      .map((n) => n.instance_id),
  );
  return {
    ...withoutNodes({ ...prev, variables: items }, dropIds),
    variables: items,
  } as T;
}

/** 应用 params 变更；若有删除则移除对应 assemble_param 节点 */
export function applyParamsChange<T extends GraphDef & { params?: ParamDef[] }>(
  prev: T,
  items: ParamDef[],
): T {
  const removed = removedNames(prev.params ?? [], items);
  if (removed.size === 0) return { ...prev, params: items };
  const dropIds = new Set(
    prev.nodes
      .filter(
        (n) =>
          n.type_id === 'assemble_param' &&
          removed.has(configName(n, 'param_name')),
      )
      .map((n) => n.instance_id),
  );
  return {
    ...withoutNodes({ ...prev, params: items }, dropIds),
    params: items,
  } as T;
}

/** 应用 returns 变更；若有删除则移除对应 return_set 节点 */
export function applyReturnsChange<T extends GraphDef & { returns?: ParamDef[] }>(
  prev: T,
  items: ParamDef[],
): T {
  const removed = removedNames(prev.returns ?? [], items);
  if (removed.size === 0) return { ...prev, returns: items };
  const dropIds = new Set(
    prev.nodes
      .filter(
        (n) =>
          n.type_id === 'return_set' &&
          removed.has(configName(n, 'return_name')),
      )
      .map((n) => n.instance_id),
  );
  return {
    ...withoutNodes({ ...prev, returns: items }, dropIds),
    returns: items,
  } as T;
}
