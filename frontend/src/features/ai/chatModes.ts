// `/` 模式选择器的预定义模式清单。
// 两类：operation 类复用后端已有意图路由；prompt 类通过 mode_hint 注入本轮 system 指令。
// 新增模式只改这个数组——前端配置化，后端无需改动（prompt 类）。

// ChatMode 分两类：
//   capability（能力）—— 注入 prompt 提升对话能力，可多选叠加；都走 chat。
//   route（流程）—— 切换到专门后端流程，一轮只能进一个，互斥独占。
export type ChatModeGroup = 'capability' | 'route';

export interface ChatMode {
  // id：`/` 后输入的匹配词与去重键
  id: string;
  // label：菜单与 chip 显示名
  label: string;
  // icon：lucide 图标名（在组件里映射）
  icon: string;
  // desc：菜单右侧说明
  desc: string;
  // group：capability 可叠加，route 独占
  group: ChatModeGroup;
  // operation：route 类透传的后端意图；hint：capability 类注入的 prompt 指令。
  operation?: string;
  hint?: string;
}

export const CHAT_MODES: ChatMode[] = [
  {
    id: 'diagram',
    group: 'capability',
    label: '画图',
    icon: 'Workflow',
    desc: '用流程图说明，文字仅作注解',
    hint: '本轮以 render_diagram 画流程图为主：把结构、流程或因果关系画成带连线的图，文字只用一两句点结论，不要长篇描述。',
  },
  {
    id: 'plan',
    group: 'capability',
    label: '计划模式',
    icon: 'ListChecks',
    desc: '只出方案与步骤，不直接落盘',
    hint: '进入计划模式：只规划方案、分解步骤、说明取舍，不要调用任何写工具（不落盘工作流/集合），等我确认后再执行。',
  },
  {
    id: 'socratic',
    group: 'capability',
    label: '苏格拉底',
    icon: 'MessagesSquare',
    desc: '反问引导，不直接给答案',
    hint: '用苏格拉底式对话：通过有针对性的反问，引导我自己想清楚问题，不要直接给出完整答案；每次只问最关键的一两个问题。',
  },
  {
    id: 'troubleshoot',
    group: 'route',
    label: '排查',
    icon: 'Stethoscope',
    desc: '采集证据 → 判断 → 建议',
    operation: 'troubleshoot',
  },
  {
    id: 'inspect',
    group: 'route',
    label: '巡检',
    icon: 'ClipboardCheck',
    desc: '生成服务器巡检工作流',
    operation: 'inspect_server',
  },
  {
    id: 'architecture',
    group: 'route',
    label: '架构分析',
    icon: 'Network',
    desc: '采集拓扑并画交互架构图',
    operation: 'analyze_architecture',
  },
  {
    id: 'workflow',
    group: 'route',
    label: '生成工作流',
    icon: 'GitBranch',
    desc: '把需求编排成可执行工作流',
    operation: 'generate_workflow',
  },
];

// matchModes 按组过滤并按查询词匹配（匹配 id 或 label）。
// group 限定触发符对应的模式组：/ → route（流程，单选），@ → capability（能力，可叠加）。
export function matchModes(query: string, group?: ChatModeGroup): ChatMode[] {
  const q = query.trim().toLowerCase();
  const list = group ? CHAT_MODES.filter((m) => m.group === group) : CHAT_MODES;
  if (!q) return list;
  return list.filter(
    (m) => m.id.includes(q) || m.label.toLowerCase().includes(q),
  );
}
