# 前端风格演进计划书：暖色科技（方向 A）

> 状态：批 1–3 全部落地（2026-06-12）
> 决策背景：保留现有 `ops-*` 暖色 token 体系（#1A1A19 底 / #D97757 强调），通过**动效 + 质感细节**做科技感升级；不换色相、不引入 framer-motion。
> 总原则：动效只服务状态语义，静止界面保持静止；除"执行中"外不允许无限循环动画；全部动效遵循 `prefers-reduced-motion`。

---

## 0. 现状基线（与代码对应）

| 事实 | 位置 |
|---|---|
| Dialog 的 `data-[state=open]:animate-in fade-in-0` 是死类——`tailwindcss-animate` 未安装，`plugins: []` | `frontend/src/components/ui/Dialog.tsx`、`frontend/tailwind.config.js` |
| 全站过渡零散：`transition-colors`×9、`transition-opacity`×2、`transition-shadow`×1，无统一时长/缓动 | 各组件 |
| RF 边映射无任何状态样式，`toRfEdge` 只产 id/source/target | `frontend/src/features/workflow/canvasMapping.ts:46` |
| 节点已有 execState 边框色（P 系列改造后用 ops-info/danger/success），但无动效 | `frontend/src/features/workflow/nodes/BaseNode.tsx` |
| 节点日志面板是普通文本列表，无 mono/光标/入场效果 | `frontend/src/features/workflow/NodeDetailPanel.tsx:341` |
| ProgressTimeline 有折叠/高亮，无入场动效 | `frontend/src/components/ui/ProgressTimeline.tsx` |
| 实时日志流（A 任务）已就绪：`execution:log` 事件逐条进 ExecutionStore | `frontend/src/features/execution/ExecutionStore.tsx` |

---

## 1. 批 1：动效基础设施（0.5 天）

### 目标
统一 motion 语言，救活已写但失效的进出场动画。

### 改动

1. **安装 `tailwindcss-animate`**，`tailwind.config.js` 注册插件。
2. **motion token**（tailwind.config.js `theme.extend`）：
   ```js
   transitionDuration: { fast: '120ms', base: '200ms', slow: '320ms' },
   transitionTimingFunction: { ops: 'cubic-bezier(0.22, 1, 0.36, 1)' }, // ease-out-quint，干脆利落
   ```
3. **Dialog 进出场**（Dialog.tsx）：开 `zoom-in-95 fade-in-0 duration-200`，关 `zoom-out-95 fade-out-0 duration-150`；Overlay 同步 fade。装上插件后现有死类直接生效，按上述微调。
4. **Dropdown / CommandPalette 进出场**：`slide-in-from-top-2 fade-in-0 duration-150`（Dropdown.tsx、CommandPalette.tsx）。
5. **统一交互过渡**：Button / Input / Select / Textarea / SidebarItem / TabBar 的 hover、focus 过渡统一为 `transition-[color,background-color,border-color] duration-fast ease-ops`；focus 环用 `transition-shadow duration-fast`。
6. **reduced-motion 兜底**（index.css）：
   ```css
   @media (prefers-reduced-motion: reduce) {
     *, *::before, *::after { animation-duration: 0.01ms !important; transition-duration: 0.01ms !important; }
   }
   ```

### 验收
- 弹窗/下拉有可感知的进出场；`npx tsc --noEmit` + `vite build` 通过；系统开启"减弱动态效果"后全部动效失效。

---

## 2. 批 2：质感与极简（1 天）

### 目标
静态界面的"科技感密度"：点阵背景、mono 数字、辉光层级、减法。

### 改动

1. **页面点阵背景**（index.css）：body 叠加 `radial-gradient(circle, #2A2A28 1px, transparent 1px)`，`background-size: 24px 24px`，透明度控制在恰好可感知（与画布 RF dots 呼应，形成"全应用都是网格"的一致性）。侧栏/弹窗等表面色容器自然遮盖，不需要逐组件适配。
2. **mono 数字**：tailwind 加 `fontFeatureSettings` 工具类或组件级 `font-mono tabular-nums`，应用于：执行耗时、节点数、ID 短串（ExecutionDetailPage、RunningBadge、ExecutionCallStack、diff 摘要）。
3. **accent 辉光（唯一允许 box-shadow 发光的场景）**：
   - token：`boxShadow: { 'glow-accent': '0 0 0 1px rgba(217,119,87,.4), 0 0 12px rgba(217,119,87,.15)' }`，同构 `glow-info` / `glow-danger`；
   - 应用：主按钮 hover、执行中节点（批 3 接管）、pending 确认卡片边框。
4. **减法（极简）**：
   - `ops-border-subtle` 从 #3A3A36 降到 #333331（分隔线弱一档）；
   - 列表行 hover 去边框变化只留底色过渡；
   - EmptyState 统一为线性图标（lucide 已有）+ 单句文案，删多余说明文字。
5. **滚动条**：宽度 10px → 8px，thumb 颜色降一档，hover 恢复——细节上更"轻"。

### 验收
- 首页/画布/执行页三屏走查：背景点阵不抢视觉、辉光只出现在 accent 元素 hover 与执行态；tsc/build 通过。

---

## 3. 批 3：执行叙事动效（1–2 天，价值最高）

### 目标
工作流执行时画布"活"起来——这是运维工具独有的科技感场景。

### 改动

1. **执行流电流边**（canvasMapping.ts + WorkflowCanvas.tsx）：
   - `toRfEdge` 增加可选状态参数；ExecutionDetailPage 的画布按 frame 状态计算每条边的视觉态：
     - 上游节点 Success 且下游 Executing → `animated: true` + `style.stroke = ops-info`（RF 自带虚线流动）；
     - 两端 Success → 实线 `ops-success`，透明度 0.7；
     - 失败链路 → `ops-danger`；
     - 未触达 → 默认弱化。
   - 编辑态画布完全不受影响（无状态参数走原样式）。
2. **执行中节点呼吸**（BaseNode.tsx + index.css）：
   - `execState === 'Executing'` 时叠加 `animate-node-breath`（自定义 keyframes：边框色 + glow-info 在 1.6s 内呼吸，复用批 2 的 glow token）；
   - Success 瞬间播一次 320ms 的 `glow-success` 脉冲（one-shot，`animation-iteration-count: 1`）。
3. **日志终端化**（NodeDetailPanel.tsx:341 区域）：
   - 容器：`font-mono text-xs bg-ops-canvas` + 行号列 + level 色（info 灰 / warn 琥珀 / error 红，色值用现有 token）；
   - 新行入场：`animate-in fade-in slide-in-from-left-1 duration-200`（仅对增量行，靠 key 控制）；
   - 跟随滚动：执行中自动滚底，用户上滚后暂停跟随（露出"回到底部"按钮）；
   - 节点执行中且暂无新日志时行尾渲染闪烁块光标 `▍`（CSS steps 动画，执行结束即移除）。
4. **ProgressTimeline 入场**：live 模式新步骤 `slide-in-from-bottom-1 fade-in duration-200`；🔧 工具行的 ✓/✗ 状态切换加 150ms 颜色过渡。
5. **AI 流式光标**（AIAssistantDialog 消息渲染）：pending 消息末尾同款 `▍` 闪烁光标，done 后移除——与日志终端光标同一个组件/类。

### 验收
- 跑一个含 3+ 节点的真实工作流：边随执行推进逐段点亮流动、执行中节点呼吸、日志逐行滑入带光标、完成后画布静止（无残留循环动画）；
- 编辑态画布与执行回放（历史执行详情）不出现执行动效残留；
- reduced-motion 下全部退化为瞬时状态切换。

---

## 4. 总览

| 批次 | 主题 | 预估 | 关键文件 |
|---|---|---|---|
| 批 1 | 动效基建：插件 + motion token + 进出场 + 统一过渡 | 0.5 天 | tailwind.config.js、Dialog/Dropdown/Button、index.css |
| 批 2 | 质感：点阵背景、mono 数字、辉光 token、减法 | 1 天 | index.css、tailwind.config.js、EmptyState、各列表 |
| 批 3 | 执行叙事：电流边、节点呼吸、终端日志、流式光标 | 1–2 天 | canvasMapping.ts、WorkflowCanvas.tsx、BaseNode.tsx、NodeDetailPanel.tsx、ProgressTimeline.tsx |

批次间无依赖阻塞，但建议顺序执行（批 3 复用批 1 的 motion token 与批 2 的 glow token）。

## 5. 不做什么

- 不引入 framer-motion / GSAP：CSS + tailwindcss-animate 覆盖全部需求，复杂编排动画目前没有场景；
- 不做扫描线、CRT 噪点、毛玻璃（backdrop-blur 在 WebView2 上有性能与渲染瑕疵风险）；
- 不改色相与品牌色：所有新增视觉均从现有 token 派生；
- 不在本计划内做第③档旧文件的 token 迁移（31 个文件的 slate 类清理仍按原节奏推进，与本计划正交）。
