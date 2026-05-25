# OpsEngine 项目完整介绍

## 1. 项目定位

**OpsEngine** 是一款**本地桌面**运维工作流编排工具，目标用户是需要把重复运维步骤可视化、可复用、可观测的团队或个人。

与典型「云端 SaaS 编排」不同，OpsEngine：

- 数据与执行均在**本机**完成（TOML 文件 + 内嵌执行引擎）；
- 编排模型接近**蓝图**（Unreal Blueprint / 节点编辑器）：**执行流（Exec）** 与 **数据流（Data）** 分离；
- 通过 **集合（Assemble）** 实现子流程复用，类似「可嵌套调用的函数模块」。

当前阶段为 **MVP + 体验迭代**：流程控制、变量、集合调用、实时执行监控、执行详情调用栈侧栏已可用；内置 **Docker**（SSH 隧道连 daemon、拉镜像、运行容器）与 **K8s 连接** 节点已落地。后续方向见 [配置环境](./environment-plan.md)、[插件平台](./plugin-platform.md)。

## 2. 核心概念

### 2.1 工作流（Workflow）

- **可独立运行**的顶层图，持久化为 `data/workflows/<id>.toml`。
- 包含：节点实例列表、连线、工作流级变量。
- 通常包含三个**系统事件节点**（各至多一个）：
  - `system_ready`：启动时触发一次主流；
  - `system_update`：可选的周期/手动增量阶段；
  - `system_over`：主流结束后的收尾流（独立超时 context）。

节点的「属于哪个阶段」**不**写在 `NodeInstance.Stage` 上，而是由**从哪个系统节点可达**推导（设计文档约定）。

### 2.2 集合（Assemble）

- **不可单独运行**，仅作为子图被引用。
- 持久化为 `data/assembles/<id>.toml`。
- 对外暴露：
  - **Params**：调用时从数据 input 传入；
  - **Returns**：在 `assemble_end` 时收集到子 Frame，再映射为调用节点的 `return_<name>` 输出。
- 内部必须有 `assemble_start`（单 exec 出）与 `assemble_end`（单 exec 入）。
- 在工作流或其它集合画布中，以动态节点类型 **`assemble:<集合ID>`** 出现（由 `app.GetNodeTypes()` 生成）。

保存集合时，应用层会 **DFS 检测循环引用**（不能直接或间接引用自身）。

### 2.3 节点类型与节点实例

| 概念 | 说明 |
|------|------|
| `NodeTypeDef` | 类型元数据：端口、ConfigSchema、NodeKind、图标等；内置节点在 `init()` 注册，集合节点运行时动态生成 |
| `NodeInstance` | 画布上的一个实例：`instance_id`、`type_id`、`config`、`position` |

### 2.4 NodeKind（节点分类）

决定端口结构与引擎调度方式：

| NodeKind | Exec 端口 | 执行方式 |
|----------|-----------|----------|
| `event` | 无 in，有 out | 作为 exec 流起点，`Execute` 常为空 |
| `action` | in + out | 沿 exec 流推进时调用 `Execute`，输出可缓存 |
| `pure` | 无 exec | 仅在下游 `Input()` 求值时按需 `Execute`，**不缓存** |
| `flow_control` | in + 多个 exec out | 引擎特判（如 `parallel`、`thread`） |

### 2.5 端口类型与连线规则

**端口类型**（`core.PortType`）：`Exec`、`String`、`Int`、`Bool`、`Dynamic`、`Any`，以及 `LinuxSshConnection`、`LinuxFileHandle`、`DockerContext`、`K8sContext` 等业务句柄。

**连线规则**（保存时后端校验 + 画布前端约束）：

| 端口 | 约束 |
|------|------|
| `exec_*` 输出 | 单出 |
| `exec_in` | 单入 |
| 数据 output | 多出 |
| 数据 input | 单入 |

拖到已有连接的 input 时，前端会断开旧边再连新边（静默替换）。

### 2.6 执行（Execution）

一次 `RunWorkflow` 产生一条 **ExecutionRecord**：

- **Snapshot**：启动时对工作流及其递归引用的全部集合做的**不可变副本**（运行中改 store 不影响本次执行）。
- **RootFrame**：主流调用栈帧；集合调用产生 **Children[callerInstanceID]** 子帧。
- **状态**：`Running` → `Success` / `Failed` / `Terminated`；终态可写入 `data/executions/`。

节点状态包括：`Idle`、`Executing`、`Success`、`Failed`、`Skipped`、`Terminated`（被 break / Stop 中断时仍在执行的节点）等。

## 3. 内置节点一览

| TypeID | 分类 | 作用 |
|--------|------|------|
| `system_ready` | event | 工作流启动入口 |
| `system_update` | event | 周期/手动 update 流（调度器） |
| `system_over` | event | 收尾阶段入口 |
| `assemble_start` | event | 集合内部起点 |
| `assemble_end` | action | 集合结束，收集 returns |
| `assemble_param` | pure | 集合参数占位（Dynamic 端口） |
| `parallel` | flow_control | 多分支并发，汇合走 `exec_out_done` |
| `thread` | flow_control | 后台 spawn exec 分支，主流走 `exec_out_continue` |
| `break` | flow_control | 取消执行上下文 |
| `varset` / `varget` | pure/action | 变量读写 |
| `print` | action | 调试日志输出 |
| `to_string` | pure | 任意类型转字符串 |
| `linux_*` | action/pure | 远程文件与命令（SSH） |
| `docker_connect` | action | SSH 隧道连远端 Docker socket，输出 `DockerContext` |
| `docker_pull` / `docker_run` / `docker_ps` / `docker_logs` / `docker_exec` / `docker_stop` / `docker_rm` / `docker_filter` | action | Docker 容器与镜像运维 |
| `k8s_connect` | action | kubeconfig 连接，输出 `K8sContext` |
| `arith` / `compare` / `logic` / `branch` / `for_loop` / `while_loop` | pure/flow | 表达式与流程控制 |

集合调用节点：`assemble:<uuid>`（非 `internal/nodes` 包，由 `app.go` 动态构造 TypeDef）。

## 4. 技术架构摘要

```
┌──────────────────────────────────────────────┐
│  Wails 桌面壳 (main.go + app.go)              │
│  · Bind: CRUD / RunWorkflow / GetNodeTypes    │
│  · Events: execution:*                        │
└────────────────────┬─────────────────────────┘
                     │
     ┌───────────────┼───────────────┐
     ▼               ▼               ▼
 internal/store  internal/engine  internal/nodes
 (TOML)          (Runtime/Frame)  (Register)
```

- **前端**：React + React Flow，通过 `wailsjs` 调用 Go，通过 `EventsOn` 订阅执行事件。
- **无独立 HTTP API**：早期 `frontend/README` 中的 REST 设计已废弃。

详见 [architecture.md](./architecture.md)。

## 5. 数据目录

| 路径 | 内容 | 版本控制 |
|------|------|----------|
| `data/workflows/*.toml` | 工作流定义 | 可提交示例 |
| `data/assembles/*.toml` | 集合定义 | 可提交示例 |
| `data/executions/*.toml` | 终态执行记录 | 通常 gitignore |
| `data/logs/` | 运行日志 | gitignore |

应用启动时（`app.startup`）会自动创建上述目录。

## 6. 用户典型工作流

1. **创建集合**（可选）：定义 params/returns、内部图、`assemble_start` → … → `assemble_end`。
2. **创建工作流**：画布上从 `system_ready` 连出主流，按需挂 `system_update`、`system_over`。
3. **添加节点**：从注册表 + 动态 `assemble:*` 拖入，配置 `config`、连数据端口。
4. **保存**：`UpdateWorkflow` / `UpdateAssemble` 触发校验后写 TOML。
5. **运行**：`RunWorkflow` → 执行列表 / 画布 / 详情页通过事件实时更新。
6. **停止**：`StopWorkflow` 取消 context，未完成的节点标 `Terminated`。

## 7. 路线图

| 文档 | 内容 |
|------|------|
| [phase8-13-plan.md](./phase8-13-plan.md) | 历史 Phase 8–13（端口重连、框选复制等） |
| [execution-ux-plan.md](./execution-ux-plan.md) | 执行列表/详情体验（**Phase 1–5 已完成**） |
| [environment-plan.md](./environment-plan.md) | 配置环境、连接/探测节点（待做） |
| [plugin-platform.md](./plugin-platform.md) | 插件生态：Lua 扩展节点与 UI 定制（待做） |

## 8. 相关文档

- [架构与调用分析](./architecture.md)
- [节点开发手册](./node-development.md)
- [源码阅读说明](./source-reading.md)
- [文档索引](./README.md)
