# 监控项画布化改造计划书：三生命周期单画布 + 自动退避修复

> 状态：设计已定稿，待后端就绪后落地（讨论产出，本轮不动代码）
> 关联：`docs/monitoring-architecture-plan.md`（监控总体）、`docs/monitoring-multi-source-plan.md`（外部源）
> 前置现状：监控已到 Phase 3/4——Conditions 表单判断（`internal/monitor/evaluator.go`）+ Troubleshoot 诊断报告（`internal/monitor/troubleshoot.go`，含可选 AI）。本改造**不动数据采集层**（collector/scheduler/datasource/planner 不变），升级"判断的表达形态"并新增"自动修复"。

---

## 0. 已定决策（用户逐轮拍板）

1. **一张画布，三类节点，三个生命周期**：数据源节点（采集）、监控指标节点（判断）、自动修复节点（修复），按生命周期分三个泳道。数据源可多个、监控指标可多个。
2. **配置驱动 + 画布只读渲染**：节点与连接全部从右侧配置面板增删/设参，左侧画布**实时渲染但不可拖拽**；点画布节点高亮对应配置项，双向联动。
3. **修复自动触发**：监控指标异常时自动触发对应修复链。
4. **退避语义**：触发间隔**用户自定义**；**下次触发的计时从"当前修复流程跑完"起算**（不是触发时刻，避免修复未完成被重复打断）；**最多触发 5 次**；**5 次用尽静默放弃**（停止重试，仅保留异常状态，不额外告警）。
5. **多对多**：指标异常输出与修复链入口多对多——一个指标可触发多个修复链，一个修复链可被多个指标触发。
6. **成败判定**：修复链跑完后**重新跑一遍指标判断**，指标恢复正常才算成功（停止重试）；仍异常算失败（进入下次退避）。"重启了但 CPU 还高"判为失败。
7. **修复链串行**：修复是串行步骤序列（重启 nginx → 清理日志 → …），节点间串行执行；一个节点可连多个下游但按顺序跑。
8. **修复流监控项专属**：不进通用工作流库、侧栏不展示，只能从监控项进入。

---

## 1. 画布与节点

### 1.1 三泳道（生命周期 = 空间直觉）

| 泳道 | 节点类型 | 颜色编码 | 数量 | 执行时机 |
|---|---|---|---|---|
| ① 数据采集 | 数据源节点 | 蓝（ops-info） | 多个 | 高频周期，采集中状态点呼吸 |
| ② 指标判断 | 监控指标节点 | 状态色（正常绿/异常红边） | 多个 | 每轮采集后判断 |
| ③ 自动修复 | 修复节点（串行链） | 橙（ops-accent，动作色） | 多个步骤 | 异常时自动触发 |

颜色即语义：蓝=数据、状态色=判断结果、橙=会真正动手的操作。节点只能在自己泳道，配置面板按段录入天然保证归属。

### 1.2 节点集

**数据源节点**（采集层，复用现有 DataSource / planner）：
- 选 kind（host.basic / host.disk / docker.containers / k8s.workloads / http.health / prometheus…）+ target + params。

**监控指标节点**（判断层，纯函数、不连服务器、不调 AI）：
- 声明引用哪个数据源 + 字段（支持单层数组展开 `filesystems[].use_percent`）+ 阈值（op + value）+ severity；
- 复杂判断用「逻辑」子项 AND/OR 组合多条件（在配置面板内编排，画布上呈现为该指标节点的内部条件，不额外占泳道）。

**自动修复节点**（执行层，复用现有执行节点：linux_exec_command / docker_* / k8s_* / linux_exec_script…）：
- 串行步骤；节点专属于监控项，不进 workflowStore。

### 1.3 连线（两种语义 + 克制动效）

- **正交路由**：React Flow `smoothstep` 边类型（横平竖直 + 转角圆角 `borderRadius: 8`），配 dagre 分层布局少交叉，整洁。
- **数据流**（源→指标）：灰实线 + 蓝色稀疏点叠加，采集时缓慢流动（~1.4s，subtle），平时近乎静止。
- **触发流**（指标异常→修复）：红色虚线，异常触发时流动加速（~0.7s），节奏明显区别于数据流；支持交叉（多对多）。
- **修复执行中**：橙实线，复用 `applyExecutionEdgeStyle` 电流流动 + 修复节点 `glow-accent` 辉光。
- **总原则**：正常态画布几乎是静的，只有"异常触发/修复执行"才有抢眼流动（守住"静止界面保持静止"）。

### 1.4 交互（配置驱动）

- 右侧配置面板分三段（数据源 / 监控指标 / 自动修复），各段可「+ 添加」节点、设参、定连接；
- 左画布只读，实时反映配置；画布点节点 ↔ 配置项高亮双向联动；
- 画布支持缩放/平移（看清全局链路），不支持拖拽改结构。

---

## 2. 数据模型

`MonitorPanel.Conditions[]` → `MonitorPanel.Flow`（统一图：含数据源 / 指标 / 修复三类节点 + 连接 + 触发边），复用 `core.NodeInstance` + `core.EdgeConfig`，节点 type 前缀区分泳道（`monsrc_* / monmetric_* / monrepair_*`）。

修复退避配置挂在修复链上：`RetryIntervalSeconds`（用户自定义）、`MaxRetries`（默认 5）。

兼容迁移（不推翻现有 Phase 3 数据）：
- 保留 `Conditions[]` 字段；提供 `Conditions → Flow` 自动转换（每条 condition 编译成 数据源→指标 链）；
- evaluator **优先用 Flow 求值，Flow 为空回退 Conditions**——存量监控项零感知。

---

## 3. 执行（三生命周期，三引擎）

| 生命周期 | 引擎 | 边界 |
|---|---|---|
| 采集 | 现有 collector/planner（不变） | 按 source_id+kind+params 去重批量采集 |
| 判断 | 新增 `internal/monitor/flowgraph.go` 轻量拓扑求值器 | 纯函数，无网络/AI；evaluator 接 Flow 分支 |
| 修复 | 复用现有 engine 执行能力，简化入口（`repair_start`，不走 system 三周期） | 注入异常上下文（触发值/evidence/target）；产 ExecutionRecord 标记 source=repair + 关联 IncidentID |

### 3.1 自动修复时序（退避语义，决策 4/6）

```
指标异常 → 自动触发修复链
  → 串行执行修复步骤 → 跑完
  → 重新跑一遍指标判断
      ├ 恢复正常 → 成功，停止（Incident 标记已自愈）
      └ 仍异常 → 失败，retry++
            ├ retry < 5 → 从「跑完时刻」起等 RetryInterval → 再次触发
            └ retry == 5 → 静默放弃（保留异常态，不额外告警）
```

关键：计时起点是修复**跑完**的时刻，不是触发时刻——修复耗时几分钟也不会被下一次触发打断。

修复执行可在异常详情页看实时日志（复用已做的终端日志/电流边/光标动效）。

---

## 4. 前端

复用 React Flow 画布基建（`WorkflowCanvas` / `canvasMapping` / `BaseNode`）+ dagre 布局：
- 监控项编辑器 = 左画布（只读渲染三泳道）+ 右配置面板（三段增删配置）；
- 三套节点视觉（数据源蓝 / 指标状态色 / 修复橙），smoothstep 正交连线 + 分语义流动动效；
- 异常详情页：触发流高亮 + 修复链执行实时日志 + 退避进度（第 N/5 次 · 跑完后等 Xm）。

---

## 5. 分阶段

| 阶段 | 主题 | 预估 | 关键改动 |
|---|---|---|---|
| R1 | 单画布 + 数据源/指标节点 + 求值器 | 4 天 | Flow 模型 + flowgraph 求值器 + evaluator 接入 + Conditions 迁移 + 左画布右配置编辑器 |
| R2 | 修复节点 + 执行入口 | 3 天 | monrepair 节点 + repair_start 简化入口 + 异常上下文注入 + ExecutionRecord 关联 |
| R3 | 自动退避闭环 | 2–3 天 | 自动触发 + 自定义间隔（跑完计时）+ 重判成败 + 5 次静默放弃 + 异常详情退避可视化 |
| R4 | 连线/状态动效打磨 | 1 天 | smoothstep 正交 + 分语义流动 + 修复执行电流/辉光 |

R1 独立可交付（判断画布化）；R2–R3 是自动修复闭环；R4 视觉收尾。

## 6. 与现有监控开发的协调点（重要）

监控是**另一条会话正在活跃开发**的领域（features/monitor/、internal/monitor/、外部源接入）。本改造触及共享文件，必须协调：

- `internal/core/monitor.go`：`MonitorPanel` 加 `Flow` 字段 + 修复退避配置（增量，不删 Conditions）；
- `internal/monitor/evaluator.go`：`Evaluate` 加 Flow 分支（向后兼容）；
- `internal/monitor/troubleshoot.go`：不动（诊断报告与自动修复执行并存，是两件事）；
- 前端 `features/monitor/`：画布编辑器是新增组件；现有 `PanelConditionsEditor` 在 R1 后由"右配置面板"替代。

建议：**等外部源接入（multi-source）告一段落后再并入**，或与那条会话明确分工冻结 `MonitorPanel` 模型与 evaluator，避免同时改。

## 7. 不做什么

- 不动采集层（collector/scheduler/datasource/planner）；
- 判断层不访问服务器、不调 AI（守住 plan §6.1 轻量边界）；
- 修复流不进通用工作流库、不走三周期模型（决策 8）；
- 不无限重试——最多 5 次后静默放弃（决策 4）；
- 不为修复流新造执行引擎——复用现有 engine，仅做简化入口适配；
- 画布不支持拖拽改结构——结构由配置面板驱动（决策 2）。
