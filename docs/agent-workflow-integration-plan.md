# Agent 与工作流模式协同改进计划书

> 状态：P1–P5 全部落地（2026-06-11）。两处与原计划的偏差：
> ① P4 未保留旧一次性生成路径的 feature flag，直接切换（execfix 仍走结构化一次性生成）；
> ② P5 保留了收窄后的 negativeKeywords——仅用于抑制巡检/架构的误命中（如"解释一下这个巡检报告"），生成类关键词路由已全部移除。
> 前置阅读：`docs/agent-runtime-architecture-optimization.md`（Agent Runtime 架构）、`docs/llm-workflow-generation-plan.md`（生成链路）
> 本计划聚焦一个主题：**打通"Agent 生成 → 引擎执行 → 结果反馈 → Agent 修复"的闭环**，并修复闭环各环节上的薄弱点。

---

## 1. 现状诊断（与代码的对应关系）

| # | 缺陷 | 代码证据 | 后果 |
|---|------|---------|------|
| 1 | 执行结果回不到 Agent | 失败反馈靠用户手工贴日志，`artifact_generate.go` 的 `isExecutionFailureReport` 用关键词嗅探；工具注册表无 `get_execution`；执行详情页无 AI 入口 | 修复质量取决于用户复述，日志不含 `exit_code`/`stderr` 等词就误路由 |
| 2 | 校验止步于"结构合法" | `engine/validate.go` 校验单例节点/端口/变量引用，但不校验节点 config 是否满足 `FieldSchema`（required/类型/选项），不校验 `env_config_id` 指向的配置是否存在 | 大量错误漏到运行时才爆，又回到缺陷 1 的薄弱反馈环 |
| 3 | 修改 = 全量 JSON 重生成，直接落盘 | `runtime/workflow.go` `handleWorkflowUpdate`：整个工作流 marshal 进 prompt，要求输出完整新 JSON，校验后 `Workflows.Save` 直接覆盖；`workflowChangeSummary` 只对比节点数 | 模型可能悄悄丢节点/改配置不可见；用户手工编辑被无确认覆盖；无版本无回滚 |
| 4 | 生成路径没有工具，Agent 设计时是"盲"的 | 工具循环（`toolloop.go`）只接在 chat/troubleshoot；`handleWorkflow` 只靠 prompt 注入静态 inventory | 模型猜路径/服务名/容器名；排障查清的事实无法结构化沉淀为工作流 |
| 5 | 意图路由脆，误判代价不对称 | `intent.go` 纯关键词规则；`negativeKeywords` 中"解释/说明"一票否决 | 误命中 generate 会真实创建资产；"先解释再生成"被拦死 |
| 6 | 工具循环无上下文预算 | `runChatToolLoop` 50 轮内 messages 无限增长；`context/budget.go` 只管 inventory | `ssh_read_file` 读大文件即可撑爆后续轮次上下文 |

施工顺序按"收益/成本比"排列：**P1 → P2 → P3 → P4 → P5**（P6 为贯穿项，随 P4 落地）。
P1、P2 互不依赖可并行；P4 依赖 P2（propose 工具复用加深后的校验）；P5 依赖 P4。

---

## 2. P1：执行失败闭环（最高优先级）

### 目标

执行失败后一键进入 AI 修复，修复上下文来自结构化的 `ExecutionRecord` 而非用户贴的文本；删除关键词嗅探。

### 改动设计

**后端**

1. `runtime.Request` 增加 `ExecutionID string` 字段；`ai.go` 的 `AIAssistantRequest` 同步增加 `execution_id`。
2. `Runtime` 新增依赖 `Executions ExecutionGetter`（`func(id string) (core.ExecutionRecord, error)`），在 `deps.go` 声明、`ai.go` 装配处注入 `engine.GetExecution`。
3. 新文件 `internal/agent/runtime/execfix.go`：
   - `buildExecutionFixContext(rec core.ExecutionRecord) string`——递归遍历 `RootFrame`（含 `Children`），收集所有 `NodeStates == Failed` 的节点：实例 ID、type_id、来自 `Snapshot.Workflow` 的节点 config、`NodeLogs` 尾部 N 条（默认 20 条，超长截断）、失败时刻的 frame `Variables`。输出为紧凑的结构化文本段。
   - `handleExecutionFix(req, session)`——加载 ExecutionRecord，定位 `Snapshot.Workflow`（即被执行的工作流），以修复上下文 + 用户补充说明为 userPrompt，复用现有 `requestArtifactDraft` + `MaterializeWorkflowUpdate` 链路。
4. `intent`：新增 `KindFixExecution`，**只由前端显式 operation 触发，不做关键词推断**。
5. 删除 `isExecutionFailureReport` / `buildExecutionFixUserPrompt` 及 `workflow.go` / `assemble.go` 中的调用分支（P1 验收通过后同 PR 删除，不留双轨）。
6. 新增只读工具 `get_execution`（`tools/builtin/`）：按 ID 返回执行摘要（状态、失败节点、日志尾部），供 chat/troubleshoot 循环随手查询。

**前端**

7. `ExecutionDetailPage`：执行状态为 Failed/Terminated 时显示「AI 修复」按钮，点击打开 `AIAssistantDialog` 并预置 `operation=fix_execution`、`execution_id`、`artifact_type=workflow`、`artifact_id=workflow_id`。
8. `AIAssistantDialog`：请求体透传 `execution_id`；会话消息流正常展示修复进度（复用现有 progress 事件，无新事件类型）。

### 验收标准

- 单测：`buildExecutionFixContext` 对含嵌套集合调用（Children 两层）、多失败节点、超长日志的 ExecutionRecord 输出正确且有界。
- 集成：构造一个必然失败的工作流（如 `linux_exec_command` 跑 `exit 1`）→ 执行 → 详情页点修复 → 模型收到的 userPrompt 包含失败节点 config 与日志 → 产出更新后的工作流。
- 回归：`isExecutionFailureReport` 相关测试删除；贴日志文本走 `update_workflow` 普通路径仍可用（兜底不丢）。

### 风险

- ExecutionRecord 可能很大（变量里有大文本）。上下文构造函数必须对每个变量值截断（如 500 字符）并标注 `…(truncated)`。

### 工作量

后端 ~1.5 天，前端 ~0.5 天，测试 ~0.5 天。

---

## 3. P2：校验加深到"可执行"级别

### 目标

让 `Materialize` 阶段拦下目前漏到运行时的三类错误；借助 `requestArtifactDraft` 已有的"校验失败喂回模型重试（3 次）"机制自动消化。

### 改动设计

1. `engine/validate.go` 新增：
   - `validateNodeConfigs(nodes, catalog)`——逐节点按 `NodeTypeDef.ConfigSchema`（`FieldSchema`）校验：required 字段非空、类型匹配（string/int/bool/select）、select 值在 options 内、Min/Max 范围。错误信息必须含**节点实例 ID + 字段 ID + 期望**（模型重试时全靠这句话定位）。
   - 校验签名需要节点目录，通过参数注入 `catalog func() []core.NodeTypeDef`，避免 engine 反依赖 store。
2. `workflow.Materialize` / `MaterializeWorkflowUpdate` 增加可选校验项：
   - `env_config_id` 类字段（`FieldSchema.Type == "env_config_select"` 等约定类型）指向的环境配置存在性，通过注入 `EnvironmentLookup` 校验；查不到时报「节点 X 的配置 Y 引用的环境配置 Z 不存在」。
3. 人工保存路径（`app.go` `UpdateWorkflow`）同样调用加深后的校验，保证 AI 与手工两条路一致；前端已有的表单校验是第一道，这里是落盘前最后一道。

### 验收标准

- 单测：缺 required 字段、select 越界、env_config_id 悬空三类草案各有用例，错误信息含节点 ID 与字段 ID。
- 集成：让模型故意产出缺字段草案（mock LLM 第一轮坏、第二轮好），验证重试链路自动修复。

### 风险

- 存量工作流可能存在校验不过的"带病数据"，`UpdateWorkflow` 突然变严会卡住用户保存。对策：人工路径先以**警告事件**形式上线（保存成功但推送校验警告），一个版本后再转硬错误；AI 路径直接硬错误（重试机制兜底）。

### 工作量

~1.5 天（含测试）。

---

## 4. P3：变更可见、可确认、可回滚

### 目标

AI 修改工作流前用户能看到改了什么，改坏了能退回去。

### 改动设计

分两步走，第一步先解决"可回滚"（便宜），第二步解决"先确认"（动交互）。

**第一步：版本快照 + 结构化 diff**

1. `WorkflowStore.Save` 落盘前，把旧版本写入 `data/workflows/history/{workflow_id}/{RFC3339 时间戳}.toml`，每个工作流保留最近 20 份，超出删最旧。集合存储同理。
2. 新增 `internal/agent/workflow/diff.go`：`DiffWorkflows(old, new) WorkflowDiff`——按节点实例 ID 对齐，输出 added / removed / config 变更字段列表 / 边增删。**仅做摘要展示用，刻意不做语义合并**。
3. `handleWorkflowUpdate` 用 `DiffWorkflows` 替换 `workflowChangeSummary`（删除后者），`ChangeSummary` 改为多行结构化摘要（如「+2 节点（linux_exec_command×2）/ 修改 n3.script / -1 边」），事件与会话消息同步携带。
4. `app.go` 新增 `ListWorkflowVersions(id)` / `RestoreWorkflowVersion(id, ts)` 绑定；前端工作流画布的菜单加「历史版本」入口（列表 + 一键恢复，恢复本身也产生一次快照）。

**第二步：AI 修改先确认（可选开关）**

5. `handleWorkflowUpdate` 校验通过后不直接 Save，而是把草案暂存到会话（`AISession` 增加 `PendingDraft` 字段，TOML 序列化），推送 `EventWorkflowPending`（携带 diff）。
6. 前端在对话流里渲染 diff 卡片 +「应用 / 放弃」；`ai.go` 新增 `ApplyAIPendingDraft(sessionID)` / `DiscardAIPendingDraft(sessionID)`。
7. 设置项 `ai_apply_mode: auto | confirm`（默认 confirm），auto 模式保留现行为（快照仍然有，可回滚）。

### 验收标准

- 单测：DiffWorkflows 覆盖增/删/改/无变化四况；快照轮替（第 21 份删最旧）。
- 集成：AI 更新 → diff 卡片内容与实际变更一致 → 放弃后存储未变 → 应用后画布刷新 → 历史版本恢复成功。

### 风险

- 用户在 AI 生成 pending 草案期间手工改了同一工作流：`ApplyAIPendingDraft` 应用前比对 `existing` 是否仍等于草案的基线版本（记录基线时间戳），不一致则提示重新生成。

### 工作量

第一步 ~1.5 天；第二步 ~2 天（前端 diff 卡片是大头）。

---

## 5. P4：生成/修改路径接入工具循环

### 目标

让 Agent 设计工作流前能先探测环境（查路径、查容器、查服务），并让排障会话能直接沉淀工作流——两件事用同一个机制解决：**把"产出工作流"本身做成工具**。

### 改动设计

1. 新增写工具 `propose_workflow` / `propose_update_workflow`（新目录 `tools/builtin/` 或独立 `tools/artifact/`，注册表区分只读/写工具两类）：
   - 入参：`draft_json`（模型产出的草案）+ `summary`（一句话变更说明）。
   - 实现：内部走 `ParseDraft → Materialize（含 P2 加深校验）→ Save（含 P3 快照/pending）`，校验失败把错误文本返回给模型（等价于把 `requestArtifactDraft` 的重试循环交给工具循环天然完成），成功返回 workflow_id + diff 摘要。
   - `ToolContext` 增加 `WorkflowSave` / `AssembleSave` 回调与 `Emitter`（发 `EventWorkflow`）。
2. `handleWorkflow` / `handleWorkflowUpdate` 重构为同一入口：构造"生成导向"的 system prompt（节点目录 + 环境 inventory + 指令：**先用只读工具核实环境事实，再调用 propose_workflow**），然后进入 `runChatToolLoop`。原一次性生成代码路径删除（`requestArtifactDraft` 保留给 inspection 两阶段使用）。
3. troubleshoot 的 system prompt 增补一句能力声明：排障结论可调用 `propose_workflow` 固化为巡检/修复工作流（用户提出时才调用，不主动）。
4. **上下文预算（缺陷 6 一并解决）**：`executeToolCall` 对 `result.Output` 设上限（默认 8KB，超出截断并标注），`runChatToolLoop` 每轮计算 messages 估算 token，超过阈值（如 24K 字符）时把最旧的 tool 消息替换为「(已省略早期工具输出)」占位。
5. 生成质量护栏：propose 工具单轮会话最多成功调用 2 次（防模型刷资产）；工具描述中写明"草案必须基于已核实的事实，未探测过的路径/服务名必须先查"。

### 验收标准

- 单测：propose_workflow 校验失败返回错误文本不抛错；上下文预算截断逻辑。
- 集成（mock LLM 脚本化回放）：「为 /opt/app 下的服务做个重启工作流」→ 模型先 `ssh_list_dir` → 再 propose → 落盘成功且事件齐全。
- 回归：inspection 两阶段路径不受影响；纯 chat 不带 propose 意图时不误产资产。

### 风险

- 这是改动量最大的一项，且模型行为不可完全脚本化。对策：保留 `operation=generate_workflow` 显式指定时的旧入口一个版本（feature flag `agent_toolloop_generate`），灰度后删除。
- 写工具突破了"注册表只读"的设计承诺，必须在 `tools/registry.go` 显式区分 `ReadOnly()` / `Mutating()` 两组，chat 默认只挂只读 + propose 两个白名单写工具，杜绝未来误挂任意写工具。

### 工作量

~3–4 天（含 mock 回放测试基建）。

---

## 6. P5：意图路由降级为"显式 + 高置信"

### 目标

P4 落地后，模型在工具循环内自己决定"答疑还是产资产"，关键词路由从"决策者"降级为"快捷方式"。

### 改动设计

1. `intent.Resolve` 收缩为三层：
   - 前端显式 operation（保留，含新增的 `fix_execution`）；
   - 高置信关键词只保留 `inspect_server`（两阶段路径仍需独立路由）与 `analyze_architecture`；
   - 其余一律进 chat 工具循环（带 propose 工具）。
2. 删除 `workflowKeywords` / `assembleKeywords` / `updateKeywords` / `negativeKeywords` 及对应分支；`troubleshootKeywords` 降级为仅用于选择排障 system prompt 的提示词开关，不再是独立 handler 路由。
3. 保留 `Result.Reason` 与事件中的意图字段用于遥测，观察一个版本期内"模型自主调用 propose 的成功率"，作为是否需要 LLM 兜底路由的依据（对应优化文档 4.3 的两阶段设想，届时再评估）。

### 验收标准

- 误路由回归用例表（至少覆盖：「解释一下这个工作流」「先解释再帮我生成」「把刚才的排查固化成工作流」「修改当前工作流加一个节点」）全部走到预期路径。

### 工作量

~1 天（主要是删代码和回归用例）。

---

## 7. 总览与里程碑

| 阶段 | 主题 | 依赖 | 预估 | 交付物 |
|------|------|------|------|--------|
| P1 | 执行失败闭环 | 无 | 2.5 天 | 「AI 修复」按钮、execfix 上下文构造、get_execution 工具、删关键词嗅探 |
| P2 | 校验加深 | 无（可与 P1 并行） | 1.5 天 | config schema / env 引用校验，AI 重试自动消化 |
| P3 | diff + 快照 + 确认 | P2（diff 依赖稳定校验） | 3.5 天 | 版本历史、结构化 diff、pending 确认模式 |
| P4 | 生成接入工具循环 | P2、P3（propose 复用其校验与落盘） | 3–4 天 | propose_workflow 工具、上下文预算、旧一次性路径删除 |
| P5 | 意图路由降级 | P4 | 1 天 | 规则收缩、回归用例表 |

总计约 12–13 个工作日。每阶段独立可发布、可验收，做完任意前缀都比现状好——即使止步 P2，闭环与校验两个最痛的点也已解决。

## 8. 不做什么（边界）

- 不引入数据库：版本历史沿用文件存储。
- 不做工作流语义级合并（多人/多会话并发编辑同一工作流的冲突解决），只做基线检测 + 提示重试。
- 不在本计划内做 Agent 主动调度执行（`run_workflow` 写工具）：执行始终由用户显式触发，避免 Agent 自主在真实环境跑任意工作流的安全问题。该能力若未来需要，必须先有 P3 的确认门 + 独立的权限协议设计。
- 不动 inspection 两阶段路径的内部实现（它是目前最稳的链路，只做接口兼容）。
