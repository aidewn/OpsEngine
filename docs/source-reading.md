# 源码阅读说明

本文提供**推荐阅读顺序**、**关键文件地图**与**调试入口**，帮助快速建立对 OpsEngine 代码库的心智模型。

## 1. 阅读前准备

1. 阅读 [introduction.md](./introduction.md) 理解 Workflow / Assemble / Exec vs Data。
2. 本地 `make dev` 跑起来，创建一个含 `print` 节点的工作流并执行一次。
3. 打开 `data/workflows/<id>.toml` 对照画布结构。

## 2. 推荐阅读路径

### 路径 A：只想改 UI（1–2 天）

```
frontend/src/main.tsx
  → App.tsx（路由）
  → pages/WorkflowCanvasPage.tsx
  → features/workflow/WorkflowCanvas.tsx
  → features/workflow/canvasMapping.ts
  → features/execution/ExecutionStore.tsx
  → features/workflow/nodes/GenericNode.tsx
```

### 路径 B：想懂执行引擎（2–4 天）

```
app.go（RunWorkflow）
  → internal/engine/engine.go（Run / runMain）
  → internal/engine/snapshot.go
  → internal/engine/runtime.go（Frame）
  → internal/engine/evaluator.go（executeFlow / evalInput）
  → internal/engine/assemble.go
  → internal/engine/parallel.go / thread.go / scheduler.go
  → internal/nodes/print/print.go（最小节点样本）
```

### 路径 C：要加新节点（半天 + 路径 B 节选）

```
internal/engine/node.go（接口）
  → internal/nodes/print/print.go
  → internal/nodes/nodes.go
  → docs/node-development.md
```

### 路径 D：校验与持久化（1 天）

```
internal/core/workflow.go / assemble.go / execution.go
  → internal/engine/validate.go
  → internal/store/workflow_store.go
  → app.go（UpdateWorkflow / checkCircularRef）
```

## 3. 仓库目录地图

```
OpsEngine/
├── main.go                 # Wails 入口，embed frontend/dist
├── app.go                  # ★ 所有对外 API
├── Makefile
│
├── internal/
│   ├── core/               # ★ 纯类型：无业务逻辑
│   │   ├── types.go        # PortType, NodeKind, NodeState, WorkflowStatus
│   │   ├── workflow.go     # WorkflowDef, EdgeConfig, VariableDef
│   │   ├── assemble.go     # AssembleDef, ParamDef
│   │   ├── node.go         # NodeTypeDef, NodeInstance, FieldSchema
│   │   └── execution.go    # ExecutionRecord, FrameState
│   │
│   ├── engine/             # ★ 执行核心
│   │   ├── engine.go       # 生命周期 runMain
│   │   ├── runtime.go      # Runtime, Frame, 状态/日志/变量 API
│   │   ├── evaluator.go      # executeFlow, evalInput/Output
│   │   ├── assemble.go     # 集合调用
│   │   ├── parallel.go     # 并发分支
│   │   ├── thread.go       # 后台线程
│   │   ├── scheduler.go    # system_update
│   │   ├── snapshot.go     # BuildSnapshot
│   │   ├── validate.go     # 保存校验
│   │   ├── registry.go     # Register / Lookup
│   │   ├── node.go         # Node 接口, ExecContext 接口
│   │   ├── exec_context.go # ExecContext 实现
│   │   ├── events.go       # Wails 事件名
│   │   └── *_test.go       # 引擎行为测试
│   │
│   ├── nodes/              # 内置节点（每类型一包）
│   │   ├── nodes.go        # import 聚合
│   │   └── <type>/         # init + TypeDef + Execute
│   │
│   └── store/              # TOML 文件读写
│
├── frontend/src/
│   ├── api/                # 薄封装 wailsjs
│   ├── types/              # 与 Go json 对齐的 TS 类型
│   ├── pages/              # 路由级页面
│   └── features/
│       ├── workflow/       # 画布、节点 UI、校验连线
│       ├── assemble/       # 集合编辑（复用 workflow 组件）
│       └── execution/      # ExecutionStore、列表、详情
│
└── docs/                   # 本文档集
```

## 4. 关键文件速查

| 问题 | 先看文件 |
|------|----------|
| 前端如何调 Go？ | `frontend/src/api/*.ts` → `frontend/wailsjs/go/main/App.ts`（dev 生成） |
| 执行事件有哪些？ | `internal/engine/events.go` |
| 节点状态存在哪？ | `runtime.go` `Frame.NodeStates` → 序列化 `core.FrameState` |
| 为何 pure 执行多次？ | `evaluator.go` `evalOutput` 无缓存分支 |
| 集合如何嵌套？ | `assemble.go` `execAssembleCall` + `Frame.Children` |
| 保存失败报端口错误？ | `validate.go` |
| 动态 assemble 节点类型？ | `app.go` `GetNodeTypes` / `assembleToNodeType` |
| 画布数据如何转换？ | `canvasMapping.ts` |
| 执行 UI 如何更新？ | `ExecutionStore.tsx` reducer + `framePath` |

## 5. 核心类型对照（Go ↔ TS）

| Go (`internal/core`) | TS (`frontend/src/types`) |
|----------------------|---------------------------|
| `WorkflowDef` | `workflow.ts` |
| `AssembleDef` | `assemble.ts` |
| `NodeTypeDef` | `nodeType.ts` |
| `ExecutionRecord` / `FrameState` | `execution.ts` |

字段名遵循 **snake_case** JSON tag，改 Go 结构后需同步 TS。

## 6. 调试技巧

### 6.1 后端日志

`app.startup` 使用 `zap.NewDevelopment()`，引擎内 `zap.L()` 与节点 `ctx.Info` 不同通道——节点日志走 **Wails 事件** 到前端。

### 6.2 断点建议（Go）

| 场景 | 位置 |
|------|------|
| 启动执行 | `engine.Run` |
| 进入节点 | `evaluator.executeFlow` 循环内 |
| 集合调用 | `execAssembleCall` |
| 数据求值 | `evalInput` / `evalOutput` |
| 取消 | `Engine.Stop` / `break` 分支 |

### 6.3 前端调试

- React DevTools 查看 `ExecutionStore` context。
- 控制台监听：在 `ExecutionStore` 的 `EventsOn` 回调临时 `console.log` payload。
- 画布：React Flow 的 `onConnect` / `isValidConnection`（`WorkflowCanvas.tsx`）。

### 6.4 测试命令

```bash
go test ./internal/engine/ -v -run TestName
go test ./internal/engine/ -count=1   # 禁用 cache 排查 flaky
```

重点测试文件：

| 文件 | 覆盖 |
|------|------|
| `flow_test.go` | exec 流基础 |
| `lifecycle_test.go` | ready/update/over |
| `assemble_test.go` | 集合调用 |
| `persistence_test.go` | 执行记录持久化 |
| `phase9_test.go` | break / update 控制 |

## 7. 常见疑惑

### Q：`frame_stack.go` 去哪了？

已移除；统一为 `Runtime` + 树状 `Frame` / `FrameState`。读旧资料时注意。

### Q：节点 `Execute` 和 `parallel.Execute` 关系？

`parallel` 包的 `Execute` 可为空；并发在 `engine.runParallel`。`flow_control` 类节点需分清**元数据在 nodes 包、逻辑在 engine 包**。

### Q：为何 `system_ready.Execute` 为空仍能跑？

事件源只提供 exec 起点；引擎从 `system_ready` 实例 ID 开始 `executeFlow`，不依赖其 `Execute` 返回值。

### Q：前端 `api/client.ts` 还有用吗？

若存在且指向 HTTP，则为历史代码；当前以 **wailsjs** 为准。以 `import ... from '../../wailsjs/go/main/App'` 为准。

## 8. 改动时的影响面

| 改动 | 通常需同步 |
|------|------------|
| `core.*` JSON 字段 | `frontend/src/types/*`、可能 TOML 示例 |
| 新 Wails 方法 | `app.go` + 前端 `api/*` + 重新 `wails dev` 生成 wailsjs |
| 新事件名/字段 | `events.go` + `ExecutionStore.tsx` |
| 新 NodeKind 规则 | `validate.go` + `WorkflowCanvas` 连接逻辑 |
| 新内置节点 | `nodes/<pkg>` + `nodes.go` + 可选 `nodeTypeMap.ts` |

## 9. 延伸阅读

- [架构与调用分析](./architecture.md) — 序列图与生命周期
- [节点开发手册](./node-development.md) — 实现步骤与检查清单
- [phase8-13-plan.md](./phase8-13-plan.md) — 待实现特性与数据模型约定
