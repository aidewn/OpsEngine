# OpsEngine 前端重设计（执行手册）

本文档是 OpsEngine 前端 UI/UX 重构的**执行基线**，供另一个 Agent 按阶段实施。

**核心目标**：
- 暗色风格（参考 Claude.ai 偏暖深色）
- Claude Desktop 风格的侧栏导航（3 tab：Chat / 工作流 / 报告）
- 无边框窗口 + 自绘标题栏
- 关键流程跳转打平、错误反馈友好、可视化可观测

参考形态：Claude Desktop 的 Agent 端。

---

## 0. 给执行 Agent 的起手指南

### 你应该先读什么
1. **本文档 §1–§7**：完整读完，理解视觉系统和 IA
2. **本文档 §8 设计决策**：所有 8 个问题已定，按此执行
3. **本文档 §9 实施分期**：按阶段顺序推进，**强烈建议阶段 0→1→2 不要跳序**
4. **现有代码起点**：
   - `frontend/src/App.tsx` — 路由根
   - `frontend/src/pages/HomePage.tsx` — 旧 6-tab 首页（即将替换）
   - `frontend/src/features/ai/AIAssistantDialog.tsx` — AI 助手（要从 Dialog 改成 Chat tab 主视图）
   - `frontend/src/components/ui/Button.tsx` / `Dialog.tsx` — 现有基础组件（要适配 token）
   - `frontend/src/features/execution/InspectionReportDialog.tsx` — 已有 Markdown 渲染（保留逻辑，适配 token）
   - `main.go` — Wails 启动入口（要改 Frameless）

### 你不应该动什么
- `internal/` Go 代码（除 `main.go` 启动参数）—— **完全不动**
- 后端的 Wails RPC 接口 —— 字段全部不变
- `frontend/wailsjs/` 自动生成 —— 不动
- 现有数据类型（`frontend/src/types/`）—— 不动，只**扩展不修改**
- React Flow 工作流画布的核心节点组件 —— 仅做暗色样式适配，不动逻辑

### 工作纪律
- 每个阶段必须 `npx tsc --noEmit` 通过才算完成
- 后端 Go 测试不允许因前端改动失败（基本不会发生，但要确认）
- 不要新增"亮色模式"切换 —— 这是单一暗色主题
- 不要因为暗色"好看"就堆叠模糊背景、大色块、半透明渐变——**简单不简陋**
- 提交粒度：一个阶段一个 commit，commit message 用本文档章节标题

### 关键命令

```bash
# 前端类型检查
cd frontend && npx tsc --noEmit

# 前端开发模式（需要 Wails CLI）
wails dev

# 后端测试（防止动错文件）
go test ./...

# 全量构建
wails build
```

---

## 1. 设计哲学

| 原则 | 含义 | 反例 |
|---|---|---|
| **暗色单一主题** | 不做亮色模式，集中维护一套 | 双主题切换 |
| **侧栏即导航** | 主任务通过左侧栏顶部 tab 切换 + 同栏列表选择 | 顶部 tab bar 横向占主区域空间 |
| **设置下沉** | 低频功能（环境配置、AI 设置）下沉到标题栏齿轮菜单 | 把"环境/设置"放在主导航占位置 |
| **就地反馈** | 用户在哪触发动作，反馈就在哪附近出现 | 跳到另一个 tab 看结果 |
| **可观测** | 长流程实时显示进度，工具调用可见、可追溯 | spinner 黑盒等几十秒 |
| **简单不简陋** | 元素少，但每个元素都有功能；不靠装饰填空间 | 大色块 + 模糊背景招贴风 |

---

## 2. 视觉系统（Design Tokens）

### 2.1 颜色（Claude.ai 偏暖深色）

#### 背景层级

| Token | 颜色 | 用途 |
|---|---|---|
| `bg-titlebar` | `#161614` | 标题栏（最浅一层，与窗口外区分） |
| `bg-canvas` | `#1A1A19` | 最底层画布（页面背景） |
| `bg-sidebar` | `#1F1F1D` | 左侧栏背景 |
| `bg-surface` | `#262624` | 列表项、卡片、对话气泡 |
| `bg-elevated` | `#2F2F2C` | 浮层（Dialog / Popover / Dropdown） |
| `bg-input` | `#1F1F1D` | 输入框背景 |
| `bg-overlay` | `rgba(0,0,0,0.6)` | Dialog 蒙层 |

#### 文字与描边

| Token | 颜色 | 用途 |
|---|---|---|
| `text-primary` | `#F5F4ED` | 标题、正文 |
| `text-secondary` | `#A8A6A1` | 次要描述、时间戳 |
| `text-tertiary` | `#6B6962` | 占位文本、禁用状态 |
| `text-inverse` | `#1A1A19` | 在浅色 chip / 主色按钮上的字 |
| `border-subtle` | `#3A3A36` | 卡片、分割线 |
| `border-strong` | `#4A4A45` | 输入框、活跃描边 |
| `border-focus` | `#D97757` | focus ring |

#### 主色与状态色

| Token | 颜色 | 用途 |
|---|---|---|
| `accent` | `#D97757` | 主操作按钮、链接、focus、active tab |
| `accent-hover` | `#C66948` | hover 态 |
| `accent-soft` | `rgba(217,119,87,0.15)` | 主色 chip 背景 |
| `success` | `#10B981` | 成功状态、工具调用完成 |
| `success-soft` | `rgba(16,185,129,0.12)` | 成功 chip 背景 |
| `warning` | `#F59E0B` | 警告、追问、过期 |
| `warning-soft` | `rgba(245,158,11,0.12)` | 警告 chip 背景 |
| `danger` | `#EF4444` | 失败、删除、报错 |
| `danger-soft` | `rgba(239,68,68,0.12)` | 错误条背景 |
| `info` | `#60A5FA` | 中性提示 |
| `info-soft` | `rgba(96,165,250,0.12)` | 信息条背景 |

#### 语义映射（强制约定）

| 含义 | 用色 |
|---|---|
| 主要 CTA（新建、运行、保存） | `accent` |
| 次要按钮、链接 | `text-primary` + `border-subtle` |
| 危险（删除、停止） | `danger` |
| 成功反馈 | `success` |
| 工具调用 / 处理中 | `info` |
| 用户决策（追问） | `warning` |

**禁止**：业务代码出现 `text-rose-600` / `bg-indigo-50` 等直接颜色类。颜色一律走 token。

### 2.2 字体

仅两条字体栈：

- `font-sans`: `"Inter", "PingFang SC", system-ui, sans-serif`
- `font-mono`: `"JetBrains Mono", "SF Mono", "Cascadia Code", monospace`

#### 字号阶梯

| Token | px | 用途 |
|---|---|---|
| `text-2xs` | 11 | 时间戳、侧栏 meta |
| `text-xs` | 12 | 列表辅助、按钮 sm |
| `text-sm` | 13 | 正文、表单 |
| `text-base` | 15 | 卡片标题、强调段 |
| `text-lg` | 18 | section 标题 |
| `text-xl` | 22 | 页面 H1 |

### 2.3 间距 / 圆角

- **间距节奏**：4 / 8 / 12 / 16 / 24 / 32 / 48 px（`space-1/2/3/4/6/8/12`）
- **圆角**：`rounded-md` = 6px（默认）、`rounded-lg` = 10px（卡片）、`rounded-full` = pill
- **阴影**：暗色环境减用阴影，用 border + elevated 背景区分层次；仅 Dialog/Dropdown 用 `shadow-2xl`

### 2.4 Tailwind 配置示例

在 `frontend/tailwind.config.ts` 中注册（执行时替换实际 RGB 值）：

```ts
export default {
  theme: {
    extend: {
      colors: {
        canvas: '#1A1A19',
        titlebar: '#161614',
        sidebar: '#1F1F1D',
        surface: '#262624',
        elevated: '#2F2F2C',
        input: '#1F1F1D',
        primary: '#F5F4ED',
        secondary: '#A8A6A1',
        tertiary: '#6B6962',
        'border-subtle': '#3A3A36',
        'border-strong': '#4A4A45',
        accent: { DEFAULT: '#D97757', hover: '#C66948', soft: 'rgba(217,119,87,0.15)' },
        success: { DEFAULT: '#10B981', soft: 'rgba(16,185,129,0.12)' },
        warning: { DEFAULT: '#F59E0B', soft: 'rgba(245,158,11,0.12)' },
        danger:  { DEFAULT: '#EF4444', soft: 'rgba(239,68,68,0.12)' },
        info:    { DEFAULT: '#60A5FA', soft: 'rgba(96,165,250,0.12)' },
      },
      fontSize: {
        '2xs': ['11px', '14px'],
        xs:    ['12px', '16px'],
        sm:    ['13px', '18px'],
        base:  ['15px', '22px'],
        lg:    ['18px', '26px'],
        xl:    ['22px', '30px'],
      },
    },
  },
}
```

执行 Agent 注意：用法上 `bg-canvas`、`text-primary`、`text-secondary` 等会与 Tailwind 默认的某些类冲突（Tailwind 自带 `text-primary` 也指字体）；如有冲突，给我们的 token 加前缀如 `ops-canvas`，但**保持名称简短**。

---

## 3. 窗口与布局

### 3.1 无边框窗口

**Wails 配置**（修改 `main.go`）：

```go
err := wails.Run(&options.App{
    Title:     "OpsEngine",
    Frameless: true,
    BackgroundColour: &options.RGBA{R: 26, G: 26, B: 25, A: 1},
    Width:  1280,
    Height: 800,
    MinWidth:  960,
    MinHeight: 640,
    Windows: &windows.Options{
        DisableFramelessWindowDecorations: false, // 保留窗口阴影 / Aero Snap
    },
    Mac: &mac.Options{
        TitleBar: mac.TitleBarHiddenInset(), // 保留 traffic lights，标题栏隐藏
    },
    // 其余参数保持现状
})
```

**前端拖拽区域**：CSS 自定义属性 `--wails-draggable: drag`（标题栏中间空白）/ `: no-drag`（所有按钮）。

### 3.2 整体布局

```
┌─ Titlebar (32px) ────────────────────────────────────────────┐
│ ☰  ⊟  🔍  ← →     [当前页 · Tab]              ⚙  _ ☐ ✕       │
├──────────────────┬───────────────────────────────────────────┤
│ Sidebar (240px)  │                                           │
│                  │                                           │
│ 💬 Chat          │                                           │
│ 🔧 工作流        │                                           │
│ 📄 报告          │            Main Content                   │
│ ──────────       │                                           │
│ + 新会话         │     (Chat 视图 / 工作流画布 / 报告详情)     │
│                  │                                           │
│ 最近             │                                           │
│ ▸ 排查 CPU 高    │                                           │
│ ▸ 生产环境巡检   │                                           │
│ ▸ ...            │                                           │
└──────────────────┴───────────────────────────────────────────┘
```

### 3.3 自绘标题栏

| 区域 | 内容 | 拖拽 |
|---|---|---|
| 最左（按钮区 ~160px） | ☰ 侧栏切换 + ⊟ 侧栏 pin + 🔍 命令面板 + ← → 历史前进后退 | `no-drag` |
| 中间（弹性） | **显示当前页**：格式 `"<选中项标题> · <Tab 名>"`，例如 `"排查 CPU 高 · Chat"` 或 `"nginx 部署 · 工作流"`；无选中项时显示 `"OpsEngine"` | `drag` |
| 最右（按钮区） | ⚙ 设置下拉 / 窗口控制（Windows 自绘 min/max/close；macOS 用系统 traffic lights） | `no-drag` |

#### ⚙ 设置下拉菜单

- AI 设置（API Key、模型、超时）→ 弹 Dialog
- 环境配置 → **跳转到全屏 view**（`/settings/environments`）
- 关于 OpsEngine → 弹小 Dialog
- 检查更新（占位 disabled）
- 退出

#### 历史前进后退 ← →

- 基于 React Router `useNavigate(-1)` / `useNavigate(1)`
- 维护一个简单的 router history stack（React Router v6 自带）

#### 命令面板 🔍 + `Ctrl+K`

详见 §5.4。

### 3.4 侧栏（Sidebar）

默认宽度 **240px**，可折叠到 **56px**（仅图标）。

#### 顶部三个 Tab（**图标 + 文字**形态）

```
💬 Chat
🔧 工作流
📄 报告
```

- 展开态：`<icon> <label>`，纵向排列
- 折叠态：仅 `<icon>`，居中
- 选中态：左侧 3px 竖线 `accent` + `bg-surface` 背景 + `text-primary`
- 未选中：`text-secondary` + hover `bg-surface/60`

#### 各 Tab 下方内容

| Tab | 侧栏内容 |
|---|---|
| **Chat** | `+ 新会话`（accent 按钮） / 最近会话列表（按 UpdatedAt 倒序） |
| **工作流** | `+ 新建`（accent 按钮） / **顶部 segment 切换 工作流 \| 集合** / 对应列表 |
| **报告** | 顶部 chip 筛选（全部 / 巡检 / 排障 / 架构） / 文档列表 |

#### 侧栏列表项形态（统一）

- 主标题：`text-sm text-primary`，单行 ellipsis
- 元信息：`text-2xs text-tertiary`，包含更新时间 + 状态 chip + 类型 chip
- 选中态：左侧 3px `accent` 竖线 + `bg-surface` 背景
- hover：`bg-surface/60`
- 右键：菜单（重命名、删除、复制 ID）

### 3.5 工作流详情页结构

工作流 tab 选中一个工作流时，主区域顶部有 **segment 切换**：

```
画布 | 执行历史
```

- **画布**：现有 WorkflowCanvas，需要适配暗色
- **执行历史**：显示该工作流的执行列表 + 单条执行的详情面板（保留现有 ExecutionDetailPage 内容，作为内嵌组件）

旧的"所有执行"汇总入口去除（命令面板可以搜执行）。

### 3.6 报告详情阅读

报告 tab 主区显示文档 Markdown。

- 右上角按钮：**全屏阅读**（点击后隐藏侧栏，再点退出）
- 标准操作按钮：复制全文 / 删除 / 重新生成（如适用）

### 3.7 环境管理（全屏 view）

通过 ⚙ → "环境配置" 跳转到 `/settings/environments`，**全屏视图**（隐藏侧栏，保留标题栏）：

- 顶部返回按钮 ← 回到原来的 tab
- 主体：环境列表 + 单环境详情双栏，或现有 EnvironmentList + EnvironmentDetailPage 路由复用

### 3.8 旧路由兼容（重要）

**全部保留 deep link**，包括 `/workflows/:id`、`/executions/:id`、`/environments/:id`、`/assembles/:id`。新布局只是把这些路由内嵌到侧栏 + 主区域的结构里。

实现方式：把现有 Page 组件作为侧栏选中后的主区域内容渲染；同时保留独立路由。

---

## 4. 核心组件库

### 4.1 标题栏组件（新建）

| 组件 | 文件 | 职责 |
|---|---|---|
| `TitleBar` | `frontend/src/components/layout/TitleBar.tsx` | 32px 自绘标题栏容器 |
| `WindowControls` | `frontend/src/components/layout/WindowControls.tsx` | min/max/close 三个按钮（平台条件渲染） |
| `NavHistoryButtons` | `frontend/src/components/layout/NavHistoryButtons.tsx` | ← → 调 router |
| `SettingsMenu` | `frontend/src/components/layout/SettingsMenu.tsx` | ⚙ 下拉菜单 |
| `TitleBarCenter` | `frontend/src/components/layout/TitleBarCenter.tsx` | 中间显示"当前页 · Tab"逻辑 |

### 4.2 侧栏组件（新建）

| 组件 | 文件 | 职责 |
|---|---|---|
| `Sidebar` | `frontend/src/components/layout/Sidebar.tsx` | 容器，管理折叠/展开 |
| `SidebarTabBar` | `frontend/src/components/layout/SidebarTabBar.tsx` | 三个 tab（💬🔧📄） |
| `SidebarList` | `frontend/src/components/layout/SidebarList.tsx` | 通用列表（传 items + renderItem） |
| `SidebarItem` | `frontend/src/components/layout/SidebarItem.tsx` | 列表项（选中态/hover/右键菜单） |
| `SidebarFilter` | `frontend/src/components/layout/SidebarFilter.tsx` | 顶部 chip / segment 筛选条 |

### 4.3 通用 UI（升级 / 新增）

| 组件 | 路径 | 状态 | 改造 |
|---|---|---|---|
| `Button` | `components/ui/Button.tsx` | 升级 | 重新映射 token；新增 `accent` variant |
| `Dialog` | `components/ui/Dialog.tsx` | 升级 | 暗色 token；增加 `size` prop（sm/md/lg/xl/fullscreen） |
| `Input` / `Textarea` | `components/ui/` | 升级 | 暗色 + `border-strong` |
| `MarkdownView` | `components/ui/MarkdownView.tsx` | 升级 | 暗色适配 + 集成 `<MermaidView>` |
| `Card` | `components/ui/Card.tsx` | **新增** | header/body/footer 三段 |
| `StatusBadge` | `components/ui/StatusBadge.tsx` | **新增** | 统一渲染执行状态/文档类型/工具状态 |
| `EmptyState` | `components/ui/EmptyState.tsx` | **新增** | 图标 + 标题 + 描述 + CTA |
| `Loader` | `components/ui/Loader.tsx` | **新增** | 统一加载态 |
| `ErrorState` | `components/ui/ErrorState.tsx` | **新增** | 统一错误态（带"重试"action） |
| `Toast` | （走 sonner） | **新增** | 全局 alert 替代 |
| `MermaidView` | `components/ui/MermaidView.tsx` | **新增** | mermaid 懒加载渲染 |
| `CommandPalette` | `components/layout/CommandPalette.tsx` | **新增** | cmdk 命令面板 |
| `Dropdown` | `components/ui/Dropdown.tsx` | **新增** | Radix Dropdown 封装 |
| `Segment` | `components/ui/Segment.tsx` | **新增** | segment 切换组件 |
| `ProgressTimeline` | `components/ui/ProgressTimeline.tsx` | **新增** | 替代 chat `<ol>` 进度列表，可折叠 |
| `ActionCard` | `components/ui/ActionCard.tsx` | **新增** | 消息内嵌操作卡片（打开工作流/查看文档/运行） |

### 4.4 强制约定

- 按钮一律 `<Button>`，禁止 `<button className=...>` 出现在业务代码
- 空 / 加载 / 错误三态走 `<EmptyState>` / `<Loader>` / `<ErrorState>`
- 反馈走 `toast.*`，禁止 `alert()` 与裸内联红框
- 颜色一律 token，禁止 `text-rose-600` 之类直名
- 图标先用 emoji 占位（性能没问题，避免依赖），阶段 6 统一切到 `lucide-react`

---

## 5. 关键交互流程

### 5.1 Chat 主路径

- **新会话**：侧栏 `+` → 主区域显示新会话表单（"环境"必填，"SSH 配置"可选）→ 提交进入对话视图
- **历史会话**：侧栏点击 → 主区域加载会话历史
- **对话视图**：消息气泡 + 底部输入框，进度面板可折叠（见 §5.5）
- **切 tab 不丢上下文**：因为 chat 是主区域第一公民，切换 tab 后回来还在

旧的 `AIAssistantDialog`（Dialog 形态）整体迁移为 `ChatView`（主区域组件），底层逻辑保留。

### 5.2 巡检流程打平

```
chat 生成巡检工作流
  └─ 消息卡片直接带「运行工作流」按钮
       └─ 点击 → 调 RunWorkflow → Toast: "已开始执行 · 查看进度"
            └─ 点 Toast → 切到工作流 tab + 选中该工作流 + segment 切到"执行历史"
                 └─ 执行完成 Toast: "执行成功 · 生成巡检报告"
                      └─ 点击 → 切到报告 tab + 选中刚生成的报告
```

具体改动：
- `ActionCard` 统一形态：根据 message 类型显示"打开工作流"/"运行"/"查看文档"
- 巡检消息加"运行此工作流"按钮 → `RunWorkflow(workflowID)` Wails 调用 + Toast
- 执行完成事件（监听 `execution:finished`）→ Toast 提示生成报告

### 5.3 Toast 反馈

引入 `sonner`，全局封装：

```ts
// frontend/src/lib/toast.ts
import { toast as baseToast } from 'sonner'

export const toast = {
  success: (msg: string, action?: { label: string; onClick: () => void }) => ...,
  error: (msg: string, description?: string) => ...,
  info: (msg: string, action?: ...) => ...,
}
```

替换所有 `alert()` 与内联红框。

### 5.4 命令面板（Ctrl+K）

**第一版只做两类**：
- **AI** — 在面板内直接发问题（在当前会话或新建会话）
- **跳转** — 工作流 / 执行 / 文档 / 会话（带搜索）

第二版（阶段 6 后）再加：操作（新建/运行）/ 历史会话快速恢复。

### 5.5 ProgressTimeline 折叠

替换现有 chat 气泡里的 `<ol>` 进度列表：

- 默认折叠为 `"5 步采集 · 3 个工具调用 [展开]"`
- 展开后按类型分组：环境加载 / 工具调用 / 模型推理 / 保存
- 工具调用单独成卡片，可点开看完整 args + output

### 5.6 Mermaid 真渲染

`MarkdownView` 识别 ` ```mermaid ` 代码块 → `<MermaidView>`：

```tsx
const MermaidView = lazy(() => import('./MermaidView'))

// in MarkdownView code block handler
if (codeLang === 'mermaid') return <Suspense fallback={<Loader />}><MermaidView source={code} /></Suspense>
```

点击 mermaid 图 → 弹全屏 Dialog 放大查看。

### 5.7 target_select 视觉强化

整条消息变 `warning` 主题（左侧竖线 + `warning-soft` 背景）；按钮区域更大；底部输入区显示"Agent 等待你的选择"banner。

### 5.8 错误处理

替换所有 `alert()` 和内联红框：
- 同步失败 → `toast.error` 带"重试"action
- 流程失败 → 对话气泡内显示带颜色的失败块 + 重试链接
- 网络 / 配置 / 模型失败按后端 `[配置错误]/[网络错误]/[模型响应错误]` 前缀 + 图标区分

---

## 6. 信息架构总览（决策汇总）

```
顶部 titlebar 32px：[☰ ⊟ 🔍 ← →]  [当前页 · Tab]  [⚙  _ ☐ ✕]

侧栏 240px（可折叠 56px）：
  💬 Chat       — 会话列表 + 新会话
  🔧 工作流      — segment（工作流|集合）+ 列表 + 新建
  📄 报告        — chip（全部|巡检|排障|架构）+ 列表
  
主区域：
  Chat:    对话视图（消息气泡 + 输入框）
  工作流:   segment（画布|执行历史）+ 对应内容
  报告:    Markdown 详情 + 全屏阅读按钮

⚙ 下拉：
  AI 设置（Dialog）
  环境配置（全屏 view，路径 /settings/environments）
  关于（Dialog）
  退出
  
旧路由全部保留 deep link 兼容。
```

---

## 7. 依赖清单

| 包 | 大小 | 用途 | 引入阶段 |
|---|---|---|---|
| `sonner` | ~10KB | Toast | 阶段 1 |
| `@radix-ui/react-dropdown-menu` | ~15KB | 设置下拉、右键菜单 | 阶段 3 |
| `mermaid` | ~700KB（懒加载） | 架构图渲染 | 阶段 5 |
| `cmdk` | ~15KB | 命令面板 | 阶段 6 |
| `lucide-react` | ~30KB（按需） | 图标统一 | 阶段 6 |
| `react-resizable-panels` | ~12KB | 侧栏拖宽（可选） | 阶段 2 末（如需） |

**不引入**：UI 框架（shadcn/mantine）、主题切换库、额外动画库。

---

## 8. 已确认设计决策

所有问题已定稿，执行 Agent 无需再问：

| # | 决策项 | 结论 |
|---|---|---|
| 1 | 侧栏三个 tab 形态 | **图标 + 文字**（展开态）；折叠态纯图标 |
| 2 | 工作流主区顶部切换形态 | **Segment** 组件（与列表 chip 风格一致） |
| 3 | 工作流和集合在侧栏的切换形态 | **顶部 Segment**（工作流 \| 集合） |
| 4 | 报告阅读 | 默认侧栏+主区，主区提供**全屏按钮**（隐藏侧栏） |
| 5 | 环境管理形态 | **全屏 view**（路径 `/settings/environments`） |
| 6 | 旧路由保留 | **全部保留 deep link** |
| 7 | 标题栏中间显示 | **显示当前页**（格式 `"<选中项标题> · <Tab 名>"`，无选中时显示 `"OpsEngine"`） |
| 8 | 命令面板第一版 | **仅"AI 直接发问"+"跳转"两类**；操作/历史会话推迟到阶段 6 后 |

---

## 9. 实施分期

每个阶段独立 commit，commit message = 阶段标题。

### 阶段 0：无边框窗口 + 自绘标题栏（约 1 天）

**目标**：应用窗口无系统边框，标题栏自绘可用。

**文件改动**：
- `main.go`：启用 `Frameless: true`、设 `BackgroundColour`、Min/Max 宽高
- 新建 `frontend/src/components/layout/TitleBar.tsx`、`WindowControls.tsx`、`NavHistoryButtons.tsx`、`SettingsMenu.tsx`、`TitleBarCenter.tsx`
- 修改 `frontend/src/App.tsx`：在所有路由外层包 `<TitleBar />`
- `frontend/src/index.css`：增加全局 CSS 自定义属性 `--wails-draggable`

**关键技术细节**：
- Windows 自绘 min/max/close：参考 `runtime.WindowMinimise/Maximise/Close`（Wails 提供）
- macOS traffic lights：用 `mac.TitleBarHiddenInset()`，左侧预留 80px 安全区
- 路由前进后退按钮：`useNavigate()(-1)` / `useNavigate()(1)`，按钮 disabled 状态根据 `window.history.length` 判断
- ⚙ 下拉**先做空菜单骨架**，菜单内容在阶段 3 填

**验收**：
- [ ] 应用启动无系统标题栏
- [ ] 拖标题栏中间空白可移动窗口
- [ ] min/max/close 按钮工作（Windows）/ traffic lights 工作（macOS）
- [ ] ← → 按钮可前进后退
- [ ] `npx tsc --noEmit` 通过
- [ ] `go test ./...` 通过（不应失败）

### 阶段 1：设计系统基础（约 1-2 天）

**目标**：所有现有页面在新 token 下能正常显示，无白底紫边等遗留。

**文件改动**：
- `frontend/tailwind.config.ts`：注册 §2.4 的颜色/字号 token
- `frontend/src/index.css`：BG 切到 `bg-canvas`
- 升级 `components/ui/Button.tsx`：用 token 重写 variantClass
- 升级 `components/ui/Dialog.tsx`、`Input.tsx`、`Textarea.tsx`、`MarkdownView.tsx`
- **新建**：`Card.tsx` / `StatusBadge.tsx` / `EmptyState.tsx` / `Loader.tsx` / `ErrorState.tsx` / `Segment.tsx`
- 引入 `sonner`，新建 `frontend/src/lib/toast.ts` 全局封装
- 全局搜索 `alert(` 替换为 `toast.error(...)` 或 `toast.success(...)`

**回归测试**：
- 所有 6 个旧 tab 内容（工作流/集合/执行/环境/文档/设置）打开后视觉无白底
- ReactFlow 画布的节点、连线、minimap 暗色适配（如需 override，加 className 即可）

**验收**：
- [ ] `npx tsc --noEmit` 通过
- [ ] 所有页面在暗色 token 下可读
- [ ] 全局无 `alert()` 调用残留（grep 验证）
- [ ] 测试 Toast：保存设置 / 删除工作流 / 错误场景

### 阶段 2：侧栏 + 3 tab 重构（约 2 天）

**目标**：核心信息架构落地。从 chat 创建巡检工作流 → 切到工作流 tab → 看到 → 切到执行历史 → 切到报告 tab，全部通过侧栏 + 主区域 segment 完成。

**文件改动**：
- 新建 `frontend/src/components/layout/Sidebar.tsx`、`SidebarTabBar.tsx`、`SidebarList.tsx`、`SidebarItem.tsx`、`SidebarFilter.tsx`
- 重写 `frontend/src/App.tsx`：移除 HomePage 6-tab，改为 `<TitleBar />` + `<Sidebar />` + `<MainContent />` 布局
- **删除** `frontend/src/pages/HomePage.tsx`（或保留作为 fallback，但默认路由改向新布局）
- **迁移** `AIAssistantDialog.tsx` 内的逻辑 → 新建 `frontend/src/features/chat/ChatView.tsx`（主区域组件）
- **重组** 文档库 UI：`OpsDocList.tsx` 拆为侧栏列表 + 主区域 `OpsDocView.tsx`
- 工作流详情主区顶部加 `<Segment>` 切换"画布"|"执行历史"
- 旧路由 `/workflows/:id`、`/executions/:id` 保留，加入到侧栏选中状态同步逻辑

**关键技术细节**：
- 侧栏当前 tab 用 URL query param 或 React Router 嵌套路由记录，刷新不丢
- 侧栏列表项选中态与 URL 双向绑定
- 标题栏 `<TitleBarCenter />` 通过 context 或 store 拿到"当前 tab + 当前选中项"

**验收**：
- [ ] 三个 tab 切换流畅，列表渲染正确
- [ ] 选中项在 URL 中可见，刷新保持
- [ ] 标题栏中间正确显示 `"<选中项> · <Tab>"`
- [ ] 侧栏可折叠到 56px 仅图标态
- [ ] 工作流 segment "画布/执行历史" 切换功能正常
- [ ] `npx tsc --noEmit` 通过

### 阶段 3：⚙ 设置下拉（环境配置全屏 view）（约 1 天）

**目标**：环境与设置从主导航解放，统一从标题栏 ⚙ 入口。

**文件改动**：
- 新建 `frontend/src/components/ui/Dropdown.tsx`（Radix Dropdown 封装）
- 完成 `SettingsMenu.tsx`：四个菜单项
- 新建 `frontend/src/pages/SettingsEnvironmentsPage.tsx`：全屏 view，路径 `/settings/environments`
- 迁移现有 `EnvironmentList.tsx` + `EnvironmentDetailPage.tsx` 内容
- 新建 `frontend/src/features/settings/AISettingsDialog.tsx`：从原 SettingsPage 抽出
- 新建 `frontend/src/components/AboutDialog.tsx`
- 全屏 view 的"返回"按钮：`navigate(-1)`

**验收**：
- [ ] ⚙ 下拉四项都能打开
- [ ] 环境配置全屏 view 显示，侧栏隐藏，标题栏保留
- [ ] AI 设置 Dialog 与之前功能等价（含 API Key 脱敏）
- [ ] 旧"环境"/"设置"tab 入口完全移除
- [ ] `npx tsc --noEmit` 通过

### 阶段 4：流程打平 + ActionCard（约 1.5 天）

**目标**：巡检全流程 1 次点击触达每阶段。

**文件改动**：
- 新建 `frontend/src/components/ui/ActionCard.tsx`：统一消息卡片形态
- 在 `ChatView.tsx` 中：根据 message 的 `workflow_id` / `doc_id` / `intent` 渲染不同 ActionCard
- 巡检消息 ActionCard 加"运行此工作流"按钮 → `RunWorkflow` + `toast.info({ action: { label: '查看进度', onClick: () => navigateToWorkflow(id) } })`
- 监听 `execution:finished` 事件 → `toast.success({ action: { label: '生成报告', onClick: () => navigateToReport(executionId) } })`
- "切到 X tab 并选中 Y" 工具函数 `navigateTo(tab, itemId)`

**验收**：
- [ ] 巡检消息有"运行此工作流"按钮，点击成功触发
- [ ] 执行完成有 Toast，点击跳转到执行详情
- [ ] 跨 tab 跳转正确选中目标项
- [ ] `npx tsc --noEmit` 通过

### 阶段 5：可视化与可观测（约 1.5 天）

**目标**：Mermaid 渲染、长对话不爆炸。

**文件改动**：
- 引入 `mermaid` 包
- 新建 `frontend/src/components/ui/MermaidView.tsx`（懒加载）
- 修改 `MarkdownView.tsx`：识别 ` ```mermaid ` 代码块替换为 `<MermaidView>`
- 点图全屏 Dialog 放大
- 新建 `frontend/src/components/ui/ProgressTimeline.tsx`：替代 chat 消息内 `<ol>`
- 工具调用 progress 项可展开看 args + output
- target_select 视觉强化：整条消息 `warning-soft` 背景 + 左侧竖线

**验收**：
- [ ] 架构报告中 Mermaid 实际渲染（不再只是源码）
- [ ] 点 Mermaid 图可全屏放大
- [ ] chat 长对话 progress 默认折叠，可展开
- [ ] target_select 视觉醒目
- [ ] `npx tsc --noEmit` 通过

### 阶段 6：命令面板 + 细节打磨（约 1.5 天）

**目标**：键盘流畅、视觉一致、质感封顶。

**文件改动**：
- 引入 `cmdk` 包
- 新建 `frontend/src/components/layout/CommandPalette.tsx`
- 全局快捷键：`Ctrl+K` 唤起命令面板、`Ctrl+I` 切到 Chat tab + 新会话焦点输入
- 命令面板包含"AI"+"跳转"两类（§5.4）
- 引入 `lucide-react`，把现有 emoji 替换为图标（侧栏 tab、按钮、消息状态）
- 全局快捷键表 `?` 唤起 Dialog
- 滚动条样式：`scrollbar-thin` + 暗色滑块
- 微动画：侧栏切换 transition、toast 淡入、segment 滑动

**验收**：
- [ ] `Ctrl+K` 唤起命令面板，可搜索跳转、可直接发 AI
- [ ] 现有 emoji 全部替换为 lucide 图标
- [ ] 全局动画顺滑（无突变）
- [ ] `npx tsc --noEmit` 通过

---

## 10. 风险与已知陷阱

| 风险 | 缓解 |
|---|---|
| **Wails Frameless 在 Windows 上窗口阴影 / Aero Snap 行为** | 测试拖到屏幕边缘是否能贴边最大化；如不行，参考 [Wails issue tracker](https://github.com/wailsapp/wails) |
| **macOS traffic lights 位置遮挡内容** | `TitleBarHiddenInset` 模式下按钮内嵌进 webview，标题栏组件左侧需预留 80px 安全区 |
| **React Flow 暗色适配** | 工作流画布默认浅色基底，节点 handle、连线、minimap 都要 override；可加 `dark` 模式 className |
| **Router 前进后退与 Wails 快捷键冲突** | 测试 `Ctrl+左/右` 是否被 Wails 拦截；如有，改用菜单按钮 |
| **首次启动无环境无 API Key** | Chat tab 在无环境时显示零状态引导卡片，三步指引 |
| **`Ctrl+K` 与系统快捷键冲突** | macOS 上 `Cmd+K` 替代 |
| **侧栏折叠态下文字 tooltip 必要性** | 折叠态用 Radix Tooltip 显示文字 label |
| **暗色下 `text-primary` 与 Tailwind 内置类冲突** | 给所有 token 加 `ops-` 前缀（如 `ops-primary`）；或直接用 `text-[#F5F4ED]` 形式（不推荐） |

---

## 11. 成功标准（验收清单）

重构完成后，下列项必须全部通过：

- [ ] 应用窗口无系统边框，标题栏自绘可拖、可前进后退、可设置
- [ ] 三个 tab（Chat / 工作流 / 报告）覆盖 95% 日常操作；环境与设置一键从标题栏可达
- [ ] 巡检流程从"用户输入需求"到"看到报告"，每一步都有就地 CTA，不需要主动切 tab
- [ ] 任意页面按 `Ctrl+K` 唤起命令面板，可跳转 / 直接发问 AI
- [ ] 架构报告的 Mermaid 直接可视化，可缩放可全屏
- [ ] 所有错误反馈都有"重试"或"如何修复"指引，不再裸 `alert()`
- [ ] 长对话的 progress 列表可折叠，工具调用细节可展开
- [ ] 所有颜色都来自 token，无散落的 `text-rose-600` 直写
- [ ] 侧栏可折叠到 56px 仅图标，工作宽屏时可释放给主区域
- [ ] 标题栏中间正确显示 "当前页 · Tab"
- [ ] 旧 deep link 全部可访问（`/workflows/:id` 等）
- [ ] `go test ./...` 全过
- [ ] `npx tsc --noEmit` 全过

---

## 12. 给执行 Agent 的建议

1. **不要试图一次性做完**。严格按阶段提交，每阶段验收通过再开下一阶段。
2. **保持后端稳定**。除 `main.go` 外不要改 Go 文件。
3. **类型先行**。每个新组件先写好 props 类型，再实现。
4. **重用 Markdown 渲染**。`MarkdownView` 已有完整 markdown 子集支持，扩展时不要从零造轮子。
5. **路由保持简单**。React Router v6 嵌套路由 + URL 参数足够，不需要引入额外状态管理库。
6. **暗色 React Flow**：先加 `dark` 主题 className 试试，必要时再写 override 样式。
7. **图标先用 emoji**。阶段 6 统一切到 lucide，不要在中间阶段就引入图标库。
8. **完成后跟用户对齐**：建议在阶段 2 / 阶段 4 完成时主动让用户跑一遍体验，再继续。
