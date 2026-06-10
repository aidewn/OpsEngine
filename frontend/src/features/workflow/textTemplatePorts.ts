// text_template 节点动态 input 端口：config.params → param_<name>
// 与 internal/nodes/text_template 及 resolveGraphPort 约定保持一致。

import type { GraphDef } from './canvasMapping';
import type { PortDef } from '@/types/nodeType';

export const TEXT_TEMPLATE_TYPE = 'text_template';

const PARAM_NAME_RE = /^[A-Za-z_][A-Za-z0-9_]*$/;

/** 从节点 config 解析参数名列表（支持 string[] 或换行字符串）。 */
export function parseTextTemplateParams(
  config: Record<string, unknown>,
): string[] {
  const raw = config.params;
  const lines: string[] = [];
  if (Array.isArray(raw)) {
    for (const item of raw) {
      if (typeof item === 'string' && item.trim()) lines.push(item.trim());
    }
  } else if (typeof raw === 'string') {
    for (const line of raw.split(/\r?\n/)) {
      if (line.trim()) lines.push(line.trim());
    }
  }
  const seen = new Set<string>();
  const out: string[] = [];
  for (const name of lines) {
    if (!PARAM_NAME_RE.test(name) || seen.has(name)) continue;
    seen.add(name);
    out.push(name);
  }
  return out;
}

/** 根据参数名生成画布 input 端口定义。 */
export function textTemplateInputPorts(paramNames: string[]): PortDef[] {
  return paramNames.map((name) => ({
    id: `param_${name}`,
    label: name,
    port_type: 'String',
    required: false,
  }));
}

/** 删除指向已移除 param_* 端口的入边。 */
export function cleanupTextTemplateEdges<T extends GraphDef>(graph: T): T {
  const allowed = new Map<string, Set<string>>();
  for (const n of graph.nodes) {
    if (n.type_id !== TEXT_TEMPLATE_TYPE) continue;
    const ports = new Set(
      parseTextTemplateParams(n.config).map((name) => `param_${name}`),
    );
    allowed.set(n.instance_id, ports);
  }
  if (allowed.size === 0) return graph;

  const filtered = graph.edges.filter((e) => {
    const ports = allowed.get(e.to.node);
    if (ports === undefined) return true;
    if (!e.to.port.startsWith('param_')) return true;
    return ports.has(e.to.port);
  });

  if (filtered.length === graph.edges.length) return graph;
  return { ...graph, edges: filtered };
}
