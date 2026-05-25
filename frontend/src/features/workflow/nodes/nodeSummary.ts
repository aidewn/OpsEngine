// 节点副标题（config 摘要）
// 用途：让画布上的节点不必点开就能看出在做什么——
//   linux_exec_command 显示 cmd 首行；math/compare 显示 "A op B (Type)"。
// 实现：按 type_id 注册的小函数表，输入 config，输出短字符串或 null。
// 没注册的节点类型 → 不显示副标题（行为零回归）。

const TRUNCATE_DEFAULT = 36;
const TRUNCATE_OPERAND = 10;

type Summarizer = (config: Record<string, unknown>) => string | null;

const summarizers: Record<string, Summarizer> = {
  // ── Linux 远程操作 ────────────────────────────────────
  linux_exec_command: (c) => firstLineTruncated(c['command']),
  linux_exec_script: (c) => firstLineTruncated(c['script']),
  linux_download_file: (c) =>
    pickTruncated([c['url'], c['dest_path']]),
  linux_find_file: (c) =>
    pickTruncated([c['path_glob'], c['path'], c['root']]),
  linux_open_file: (c) => pickTruncated([c['path']]),
  linux_file_write: (c) => pickTruncated([c['path']]),
  linux_file_append: (c) => pickTruncated([c['path']]),
  linux_file_replace: (c) => pickTruncated([c['path']]),

  // ── 算术 ─────────────────────────────────────────────
  math_add: (c) => arithSummary(c, '+'),
  math_sub: (c) => arithSummary(c, '−'),
  math_mul: (c) => arithSummary(c, '×'),
  math_div: (c) => arithSummary(c, '÷'),
  math_mod: (c) => arithSummary(c, '%'),

  // ── 比较 ─────────────────────────────────────────────
  compare_eq: (c) => compareSummary(c, '=='),
  compare_ne: (c) => compareSummary(c, '!='),
  compare_lt: (c) => compareSummary(c, '<'),
  compare_gt: (c) => compareSummary(c, '>'),
  compare_le: (c) => compareSummary(c, '<='),
  compare_ge: (c) => compareSummary(c, '>='),
};

// nodeSummary 返回该节点 instance 的副标题；缺省 null（不渲染）
export function nodeSummary(
  typeId: string,
  config: Record<string, unknown>,
): string | null {
  const fn = summarizers[typeId];
  if (!fn) return null;
  const out = fn(config);
  if (!out) return null;
  return out;
}

// ── helpers ────────────────────────────────────────────

function asString(v: unknown): string {
  if (typeof v === 'string') return v;
  if (typeof v === 'number' || typeof v === 'boolean') return String(v);
  return '';
}

function truncate(s: string, max = TRUNCATE_DEFAULT): string {
  if (s.length <= max) return s;
  return s.slice(0, max - 1) + '…';
}

function firstLineTruncated(v: unknown, max = TRUNCATE_DEFAULT): string | null {
  const s = asString(v).trim();
  if (!s) return null;
  const first = s.split(/\r?\n/, 1)[0] ?? '';
  return truncate(first, max);
}

// 从候选 config 值里挑第一个非空字符串
function pickTruncated(values: unknown[], max = TRUNCATE_DEFAULT): string | null {
  for (const v of values) {
    const s = asString(v).trim();
    if (s) return truncate(s, max);
  }
  return null;
}

// arithSummary 形如 "5 + 3 (Int)"；任一端未配默认值则用占位 "A"/"B"
function arithSummary(config: Record<string, unknown>, op: string): string {
  const a = operandLabel(config, 'a');
  const b = operandLabel(config, 'b');
  const t = typeOf(config);
  return `${a} ${op} ${b}${t ? ` (${t})` : ''}`;
}

// compareSummary 与 arithSummary 同结构；保留独立函数以便后续区别化
function compareSummary(config: Record<string, unknown>, op: string): string {
  return arithSummary(config, op);
}

// operandLabel 优先 <port>_default 配置；否则用占位大写字母
function operandLabel(config: Record<string, unknown>, portId: string): string {
  const s = asString(config[`${portId}_default`]).trim();
  if (s) return truncate(s, TRUNCATE_OPERAND);
  return portId.toUpperCase();
}

function typeOf(config: Record<string, unknown>): string {
  const t = asString(config['var_type']).trim();
  return t;
}
