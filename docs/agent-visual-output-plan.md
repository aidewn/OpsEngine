# Agent 可视化输出计划书：画图为主，文字为辅

> 状态：V1+V2+V3 全部落地（2026-06-12）。
> V3 实现偏差：因果链图与 render_diagram 合并为同一机制——troubleshoot 用 render_diagram 画"事实→判断→建议"（节点 kind=fact/judgment/suggestion），不再单独解析 JSON 文本块，更可靠且一套渲染器覆盖全部画图需求。
> inspection_report 卡顺延：巡检报告数据源在 report 包、走工作流执行而非 chat 工具循环，与视图协议链路不同；模型在排障/解释中已可用 render_diagram 表达，专门卡价值有限，暂不做。
> 已定决策：① 结构化视图协议 + 前端精修组件库（不让模型生成 UI）；② 架构图主展示用 React Flow 交互拓扑，Mermaid 降级为文档导出；③ 视图由"工具 → 视图"固定映射起步；④ 推理型输出同样图优先（V3 专项）；⑤ 状态卡为执行时刻快照，持续刷新留给未来的监控功能。
> 总原则：漂亮与动效是一次性工程投入，模型只是数据源；视图数据不回流模型上下文（省 token），模型只消费工具的文本摘要。

---

## 0. 架构总览

```
工具执行 ──► tools.Result{ Output(给模型), View(给前端) }
                              │
runtime ──► EventView 事件（实时） + AISessionMessage.Views 持久化（回放）
                              │
前端 ViewRenderer 注册表 ──► 按 view.kind 渲染精修组件（卡片/拓扑/链路图）
```

关键设计：
- **View 与 Output 分离**：`Output` 仍是喂给模型的紧凑文本（现状不变）；`View` 是只给前端的结构化 JSON。两者来自同一次采集，无重复开销。
- **持久化回放**：视图数据存进会话消息（`AISessionMessage.Views []ViewPayload`），历史会话打开时卡片照常渲染。
- **降级兜底**：前端遇到未知 `kind`（旧版本会话/未来新增视图）渲染为折叠的 JSON 块，永不白屏。

## 1. V1：视图协议 + 两个最高价值视图（3–4 天）

### 1.1 后端协议

1. `tools.Result` 增加可选字段：
   ```go
   // ViewPayload 是工具产出的前端可视化载荷；nil 表示该工具无视图。
   type ViewPayload struct {
       Kind  string `json:"kind"`  // host_status / topology / container_list ...
       Title string `json:"title"` // 卡片标题，如 "web-1 (192.168.1.10)"
       Data  string `json:"data"`  // 视图专属 JSON
   }
   ```
2. `runtime.Event` 增加 `EventView` 类型（携带 ViewPayload + 关联 RequestID）；`executeToolCall` 在工具返回 View 时 Emit。
3. `core.AISessionMessage` 增加 `Views []ViewPayload`；回合结束把本轮收集的视图挂到 assistant 消息上持久化。
4. `ai.go` 的 `AIAssistantEvent` 透传 view 字段。

### 1.2 前端协议

5. `features/ai/views/` 新目录：`ViewRenderer.tsx`（kind → 组件注册表 + 未知 kind 的 JSON 折叠兜底）；Bubble 在 progress 时间线之后、正文之前渲染 Views。
6. 事件 reducer：`view` 事件追加到 pending 轮次；落库后从 message.views 渲染。

### 1.3 视图一：host_status 主机状态卡（数据源 ssh_inspect）

- 数据：hostname、os、uptime、CPU 使用率与核数、内存用量/总量、磁盘分区列表（挂载点/用量/容量）、top 进程 N 条。
- 视觉：三个环形仪表（CPU/内存/最满磁盘）+ 磁盘分区横条 + 进程小表。
- 动效（复用风格批 1/2 的 token）：仪表首帧从 0 充能到目标值（600ms ease-ops）、数值滚动、用量 >85% 的仪表用 danger 色 + 一次性脉冲提醒。
- 卡片右上角「重新采集」按钮：sendMessage 复用现有轮次机制重跑 ssh_inspect（快照语义，决策 ⑤）。

### 1.4 视图二：topology 交互拓扑（数据源 analyze_architecture）

- 后端：`architecture.TopologyGraph` 已是结构化节点/边——新增 `graph_json` 序列化（节点 kind：server/port/container/process），随 EventView 发出；Mermaid 路径保留但降级为 OpsDoc 文档内容。
- 前端：`TopologyView`——只读 React Flow 实例（与工作流画布同库不同 nodeTypes）：
  - 服务器为分组容器节点，端口/容器/进程为子节点，自动布局（dagre 或简单分层算法）；
  - hover 节点高亮关联边（电流样式复用 `applyExecutionEdgeStyle` 的视觉语言）；
  - 容器节点 running 状态点呼吸（复用 `animate-pulse` 语义）；
  - 点击节点 → 侧浮层显示原始采集详情。
- 卡片支持放大：点击展开为全屏 Dialog（复用现有 Dialog fullscreen size）。

### V1 验收
- 在绑定 SSH 的会话里问"看下这台机器的状态"→ 模型调 ssh_inspect → 对话流里出现仪表卡（充能动画）+ 模型一句话解读；
- "分析这个环境的架构" → RF 交互拓扑卡 + hover 高亮 + 全屏展开；
- 关闭重开会话，两张卡从持久化数据原样回放；
- 未知 kind 渲染 JSON 兜底不报错；tsc/build/go test 全绿。

## 2. V2：数据卡全覆盖（约 2 天）

按"工具 → 视图"固定映射补齐：

| 工具 | 视图 kind | 视觉要点 |
|---|---|---|
| docker_list_containers | container_list | 状态点（running 呼吸/exited 灰/restarting 琥珀）+ 镜像/端口列 |
| k8s_list_pods / workloads | k8s_workloads | 命名空间分组 + ready 比例徽章 + 重启次数告警色 |
| jenkins_list_jobs | job_list | 最近构建结果色条 |
| get_execution | execution_summary | 状态横幅 + 失败节点列表（点击跳执行详情页） |
| 巡检完成（inspection） | inspection_report | ok/warn/fail 三色统计环 + 分级结果列表 |

每张卡 ≤1 屏高度，超出折叠"展开全部"；全部遵循 reduced-motion。

## 3. V3：推理可视化——"画图为主"落到解释类输出（2–3 天）

数据卡解决了"状态展示"，但决策 ④ 要求根因分析、方案解释这类**推理输出**也图优先。推理文本无法确定性自动成图，用两条机制覆盖：

### 3.1 排障结构化输出 → 因果链图

- troubleshoot 路径的 system prompt 已强制"事实/判断/建议"三段——升级为要求模型在结尾输出一个受 schema 约束的 JSON 块：
  ```json
  {"facts":[{"id":"f1","text":"磁盘 /var 已满 98%"}],
   "judgments":[{"id":"j1","text":"日志未轮转导致","from":["f1"],"confidence":"high"}],
   "suggestions":[{"id":"s1","text":"配置 logrotate","from":["j1"],"risk":"low"}]}
  ```
- 后端解析（失败则纯文本降级，不报错）→ `troubleshoot_chain` 视图：三列因果链图（事实→判断→建议），边表示推导关系，confidence/risk 用色彩编码；
- 正文文字压缩为一句话结论（prompt 约束"图已表达的内容不要在文字里复述"）。

### 3.2 `render_diagram` 工具——模型主动画图

- 新增只读工具 `render_diagram(spec_json)`：受限图 schema（nodes[{id,label,kind}], edges[{from,to,label}], groups 可选），由前端通用 `DiagramView`（RF 只读 + 自动布局）渲染；
- 模型在解释部署方案、流程顺序、组件关系时**主动调用**画示意图——这是"画图为主"对自由解释类输出的实现方式：图是模型画的，但渲染器是受控的（无 HTML 注入面，风格永远一致）；
- chat/troubleshoot 的 system prompt 增加全局表达规范：**"涉及结构、流程、因果、对比时优先调用 render_diagram；文字只做图的注解，单段不超过 3 句"**——这条规则是把"以画图为主"固化进模型行为的关键；
- 防滥用：每轮 render_diagram 上限 3 次；节点数上限 30。

### V3 验收
- "为什么这台机器磁盘满了" → 因果链图 + 一句话结论，文字量显著少于现状；
- "解释一下蓝绿部署和滚动更新的区别" → 模型画对比示意图 + 简短注解；
- JSON 解析失败时静默降级为纯文本，无错误弹窗。

## 4. 总览

| 阶段 | 主题 | 预估 | 关键改动 |
|---|---|---|---|
| V1 | 视图协议 + 主机状态卡 + RF 拓扑 | 3–4 天 | tools.Result / EventView / Views 持久化 / ViewRenderer / HostStatusCard / TopologyView |
| V2 | 数据卡全覆盖（容器/K8s/Jenkins/执行/巡检） | 2 天 | 各工具 View 装配 + 5 个卡片组件 |
| V3 | 推理可视化（因果链 + render_diagram） | 2–3 天 | troubleshoot schema / DiagramView / 表达规范 prompt |

## 5. 不做什么

- 不让模型输出 HTML/JSX/SVG 源码——一切视图经类型化 JSON + 受控渲染器；
- 不做持续刷新/实时曲线（决策 ⑤，属未来监控功能；卡片留"重新采集"手动入口）；
- 不把视图 JSON 回喂模型上下文（模型只看 Output 文本摘要，token 成本零增长）；
- V1/V2 不引入图布局库以外的新依赖（自动布局若简单分层不够再评估 dagre，~12KB）。
