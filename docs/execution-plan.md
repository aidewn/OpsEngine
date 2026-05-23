# OpsEngine 执行引擎实施计划

> 让最小工作流（system_ready → print "Hello World"）跑起来，然后逐步扩展。

---

## 0. 总览

### 设计原则

- **UE Blueprint 执行模型**：单线 exec 流推进 + lazy pull 数据流
- **不可变快照**：每次运行创建独立 `ExecutionRecord`，包含工作流和所有引用集合的快照副本
- **节点开发友好**：实现一个新节点 = 一个 Go 文件 + 一行 import
- **事件推送**：执行状态/日志通过 Wails `EventsEmit` 实时推送到前端

### 总体架构

```
┌────────────────────────────────────────────────┐
│ 前端（React）                                   │
│ ┌──────────┐  ┌──────────────┐  ┌────────────┐ │
│ │ Run 按钮 │→ │ ExecutionTab │←─│ EventStore │ │
│ └──────────┘  │ + Canvas只读 │  └────────────┘ │
│       ↓       └──────────────┘       ↑         │
└───────│──────────────────────────────│─────────┘
        │ Wails 调用                  │ Wails 事件
┌───────▼──────────────────────────────│─────────┐
│ 后端（Go）                            │         │
│  app.go                               │         │
│   ├─ RunWorkflow → engine.Run()       │         │
│   └─ StopWorkflow                     │         │
│                                       │         │
│  internal/engine/                     │         │
│   ├─ Engine（管理多次运行）            │         │
│   ├─ Runtime（单次运行的状态/帧栈）    │         │
│   ├─ Registry（节点注册表）           │         │
│   └─ Snapshot（构造不可变副本）       │         │
│       │                               │         │
│       ▼                               │         │
│  internal/nodes/                      │         │
│   ├─ print/      ────────────────────┘         │
│   ├─ varget/   ctx.Log() / state 变更           │
│   ├─ varset/                                    │
│   └─ ...                                        │
└─────────────────────────────────────────────────┘
```

---

## 1. 核心数据结构

### 1.1 ExecutionRecord（执行记录）

```go
// internal/core/execution.go
package core

import "time"

type ExecutionRecord struct {
    ID         string                  `json:"id"`           // 执行 ID（UUID）
    WorkflowID string                  `json:"workflow_id"`  // 关联工作流 ID
    Snapshot   ExecutionSnapshot       `json:"snapshot"`     // 不可变快照
    Status     WorkflowStatus          `json:"status"`       // running/success/failed/terminated
    StartedAt  time.Time               `json:"started_at"`
    FinishedAt *time.Time              `json:"finished_at,omitempty"`
    NodeStates map[string]NodeState    `json:"node_states"`  // key = instance_id
    NodeLogs   map[string][]LogEntry   `json:"node_logs"`
    Variables  map[string]any          `json:"variables"`    // 主 frame 的变量终态
    Error      string                  `json:"error,omitempty"`
}

// ExecutionSnapshot 启动时打的快照，包含工作流和所有递归引用的集合
type ExecutionSnapshot struct {
    Workflow  WorkflowDef              `json:"workflow"`
    Assembles map[string]AssembleDef   `json:"assembles"` // key = assemble ID
}

type LogEntry struct {
    Time    time.Time `json:"time"`
    Level   string    `json:"level"`   // info/warn/error
    Message string    `json:"message"`
}
```

### 1.2 Frame（调用栈帧）

```go
// internal/engine/runtime.go
type Frame struct {
    AssembleID string         // 当前帧对应的集合 ID；主工作流为空
    Variables  map[string]any // 本帧的变量作用域
    Params     map[string]any // 调用时传入的参数（仅 assemble frame）
    Returns    map[string]any // 待返回给调用者的值
    Parent     *Frame         // 父帧
}
```

---

## 2. 引擎接口

### 2.1 Node 接口（节点开发者面对的契约）

```go
// internal/engine/node.go

// Node 节点逻辑实现，每个节点类型对应一个 Node 实例
type Node interface {
    // TypeDef 返回节点元信息（端口、config schema 等）
    TypeDef() core.NodeTypeDef

    // Execute 执行节点逻辑；Pure 节点也实现此方法，但由 evaluator 按需调用
    Execute(ctx ExecContext) (Outputs, error)
}

// Outputs 节点输出端口的值，key = output port ID
type Outputs map[string]any

// ExecContext 节点执行时拿到的上下文
type ExecContext interface {
    Context() context.Context  // 用于取消

    // Input 拉取某个 input 端口的值
    // 内部按需求值上游 pure 节点 / 读已执行 action 的 output cache
    // 未连线时返回 (零值, false)
    Input(portID string) (any, bool)

    // Config 读节点 config 字段
    Config(fieldID string) any
    ConfigString(fieldID string) string
    ConfigInt(fieldID string) int64
    ConfigBool(fieldID string) bool

    // 变量读写（当前 frame 作用域）
    GetVariable(name string) (any, bool)
    SetVariable(name string, value any)

    // 日志（推送到前端的 execution:log 事件）
    Info(format string, args ...any)
    Warn(format string, args ...any)
    Error(format string, args ...any)
}
```

### 2.2 Registry 注册表

```go
// internal/engine/registry.go

var registry = map[string]Node{}

// Register 节点 init() 时调用注册自己
func Register(n Node) {
    typeID := n.TypeDef().TypeID
    if _, exists := registry[typeID]; exists {
        panic("节点类型重复注册: " + typeID)
    }
    registry[typeID] = n
}

// Lookup 按 typeID 查节点
func Lookup(typeID string) (Node, bool) {
    n, ok := registry[typeID]
    return n, ok
}

// AllTypeDefs 返回所有静态注册节点的 TypeDef
// 集合调用节点（assemble:<id>）由 app.GetNodeTypes 动态拼接
func AllTypeDefs() []core.NodeTypeDef {
    defs := make([]core.NodeTypeDef, 0, len(registry))
    for _, n := range registry {
        defs = append(defs, n.TypeDef())
    }
    return defs
}
```

### 2.3 Engine API（Wails 绑定层）

```go
// internal/engine/engine.go

type Engine struct {
    ctx           context.Context           // Wails 主 context
    workflowStore *store.WorkflowStore
    assembleStore *store.AssembleStore
    runs          map[string]*Runtime       // key = execution ID
    mu            sync.RWMutex
}

func (e *Engine) Run(workflowID string) (executionID string, err error)
func (e *Engine) Stop(executionID string) error
func (e *Engine) Get(executionID string) (*Runtime, bool)
func (e *Engine) List() []*Runtime
func (e *Engine) ListByWorkflow(workflowID string) []*Runtime
func (e *Engine) Remove(executionID string) error  // 仅终态可移除
```

---

## 3. Wails 事件契约

| 事件名 | Payload | 触发时机 |
|--------|---------|----------|
| `execution:started` | `{ executionID, workflowID, snapshot }` | 开始运行 |
| `execution:status` | `{ executionID, status }` | 整体状态变化 |
| `execution:node` | `{ executionID, nodeID, state, errorMsg? }` | 节点状态变化 |
| `execution:log` | `{ executionID, nodeID, time, level, message }` | 每条日志 |
| `execution:variable` | `{ executionID, frameID, name, value }` | 变量值变化 |
| `execution:finished` | `{ executionID, status, error? }` | 结束 |

前端在根组件订阅一次，分发到 `ExecutionStore`（Context + reducer）。

---

## 4. 校验规则

### 4.1 单例节点

| 节点 | 适用范围 | 至多 |
|------|----------|------|
| system_ready / system_update / system_over | Workflow | 各 1 个 |
| assemble_start / assemble_end | Assemble | 各 1 个 |

- 前端 `AddNodeDialog` 用 `isInternalNodeType` 过滤，本来就不可加（再加一道保险：列表里也过滤已存在的）
- 后端 `UpdateWorkflow` / `UpdateAssemble` 保存前校验，违反则 reject

### 4.2 Exec 边连接

- 每个 `exec_out` 最多 1 条出边
- `exec_in` 可接收多条入边
- 前端 `isValidConnection` 拦截违规连接（提示「Exec 输出端口只能连一条线」）
- 后端 `UpdateWorkflow` / `UpdateAssemble` 保存前校验

### 4.3 节点不可删除

- system_ready / system_update / system_over / assemble_start / assemble_end 不可删除
- 前端 `onNodesDelete` 过滤掉这些类型 + toast 提示

### 4.4 循环引用（已实现）

`UpdateAssemble` 时 DFS 检测集合是否引用自身。✓

---

## 5. 节点清单

| TypeID | NodeKind | 适用 | Phase | 说明 |
|--------|----------|------|-------|------|
| `system_ready` | event | Workflow | 0 注册 / 2 实装 | 启动入口，无 exec_in |
| `system_update` | event | Workflow | 0 注册 / 6 实装 | 周期触发 |
| `system_over` | event | Workflow | 0 注册 / 6 实装 | 终止入口 |
| `assemble_start` | event | Assemble | 0 注册 / 4 实装 | 集合入口 |
| `assemble_end` | action | Assemble | 0 注册 / 4 实装 | 集合出口（设置 Returns） |
| `assemble_param` | pure | Assemble | 0 注册 / 4 实装 | 输出 Params |
| `print` | action | 通用 | 2 实装 | 打印消息 |
| `var_get` | pure | 通用 | 2 实装 | 读变量 |
| `var_set` | action | 通用 | 2 实装 | 写变量 |
| `parallel` | action | 通用 | 0 注册 / 5 实装 | 多 exec_out 并行 |
| `thread` | action | 通用 | 0 注册 / 5 实装 | spawn 后台线程 |
| `assemble:<id>` | action | 工作流/集合 | 0 动态生成 / 4 实装 | 集合调用 |

---

## 6. 分阶段实施

### Phase 0：节点注册框架

**目标**：把 `app.go` 里硬编码的 `builtinNodeTypes` 拆成 `internal/nodes/<name>/` 每个节点一个包。本阶段只重构，不改运行时行为。

**新增文件**：
```
internal/engine/node.go          # Node + ExecContext 接口（Execute 暂可空实现）
internal/engine/registry.go      # 注册表
internal/nodes/nodes.go          # 聚合所有节点的 anonymous import
internal/nodes/print/print.go
internal/nodes/varget/varget.go
internal/nodes/varset/varset.go
internal/nodes/system_ready/system_ready.go
internal/nodes/system_update/system_update.go
internal/nodes/system_over/system_over.go
internal/nodes/assemble_start/assemble_start.go
internal/nodes/assemble_end/assemble_end.go
internal/nodes/assemble_param/assemble_param.go
internal/nodes/parallel/parallel.go
internal/nodes/thread/thread.go
```

**修改文件**：
- `app.go`：`GetNodeTypes` 改用 `engine.AllTypeDefs()` + 动态拼集合节点；删除 `builtinNodeTypes` 常量；加入 `import _ "OpsEngine/internal/nodes"`

**Node 文件骨架**（示例 print）：
```go
package print

import (
    "OpsEngine/internal/core"
    "OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

type Node struct{}

func (Node) TypeDef() core.NodeTypeDef {
    return core.NodeTypeDef{
        TypeID:      "print",
        DisplayName: "打印",
        // ...（从原 builtinNodeTypes 拷过来）
    }
}

func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
    return nil, nil  // Phase 2 实装
}
```

**验收**：
- `go build ./...` 通过
- 启动后 `GetNodeTypes` 返回的列表跟以前一致（前端画布渲染完全不变）
- 新增 parallel / thread 节点出现在 AddNodeDialog 里

---

### Phase 1：约束校验

**目标**：补全前后端校验，防止违规的连线和节点存在。

**修改文件**：
- `app.go`：`UpdateWorkflow` 加 `validateWorkflow`、`UpdateAssemble` 加 `validateAssemble`
- `internal/engine/validate.go`（新增）：单例 + exec_out 单连接校验
- `frontend/src/features/workflow/WorkflowCanvas.tsx`：`isValidConnection` 加 exec_out 边数限制；`onNodesDelete` 过滤系统/集合节点
- `frontend/src/features/workflow/WorkflowCanvas.tsx`：`addNodeToGraph` / `addNodeWithEdge` 用 NodeTypeDef.ConfigSchema 的 default 填充 config

**校验逻辑骨架**：
```go
// internal/engine/validate.go
func ValidateWorkflow(wf core.WorkflowDef) error {
    // 单例
    if c := countByType(wf.Nodes, "system_ready"); c > 1 {
        return fmt.Errorf("system_ready 只能存在一个")
    }
    // ... system_update / system_over 同
    // exec_out 单连接
    return validateExecEdges(wf.Nodes, wf.Edges)
}
```

**验收**：
- 拖第二条 exec_out 边时前端拒绝并 toast
- 后端 PUT 一个含 2 个 system_ready 的工作流时返回错误
- 系统节点的 ✕ 删除按钮变灰或被拦截

---

### Phase 2：引擎核心 + Hello World

**目标**：跑通 `system_ready → print "Hello World"`，前端能实时看到节点状态变化和日志。

**新增文件**：
```
internal/core/execution.go        # ExecutionRecord / LogEntry
internal/engine/runtime.go        # Runtime + Frame
internal/engine/snapshot.go       # 构造快照
internal/engine/evaluator.go      # pure 节点求值
internal/engine/engine.go         # Run / Stop
internal/engine/events.go         # 封装 Wails EventsEmit
```

**修改文件**：
- `app.go`：新增 `engine *engine.Engine`，`startup` 中创建；绑定 `RunWorkflow` / `StopWorkflow` / `ListExecutions` / `GetExecution` / `DeleteExecution`
- `internal/nodes/print/print.go`：实装 Execute（读 message，写日志）
- `internal/nodes/varget/varget.go`：实装 Execute（从 frame.Variables 读）
- `internal/nodes/varset/varset.go`：实装 Execute（写 frame.Variables + 推 `execution:variable`）
- `internal/nodes/system_ready/system_ready.go`：实装 Execute（空操作，仅作为流起点）

**关键算法**：
```go
// executeFlow 从一个 exec 起点开始单线推进
func (r *Runtime) executeFlow(startNodeID string) error {
    cur := startNodeID
    for cur != "" {
        select {
        case <-r.ctx.Done():
            return r.ctx.Err()
        default:
        }

        node := r.findNode(cur)
        r.setNodeState(cur, Executing)

        outputs, err := r.execNode(node)
        if err != nil {
            r.setNodeState(cur, Failed)
            return err
        }
        r.setNodeState(cur, Success)
        r.cacheOutputs(cur, outputs)

        cur = r.findNextExec(cur)  // 沿 exec_out 找下一个
    }
    return nil
}
```

**Pure 求值**：
```go
// evalInput 拉取某个节点的某个 input
// - 找连到该 input 的边
// - 如果源节点是 action：从 output cache 读
// - 如果源节点是 pure：递归调用 evalPure
// - 未连线：返回零值
func (r *Runtime) evalInput(nodeID, portID string) (any, bool)
```

**验收**：
- 用户点「▶ 运行」→ 后端返回 executionID
- 后端在事件流里推：started → node(ready, Executing) → node(ready, Success) → node(print, Executing) → log(print, "Hello World") → node(print, Success) → finished
- 前端不一定有 UI，但 Wails 控制台能看到事件

---

### Phase 3：前端执行 UI

**目标**：让 Phase 2 的事件真正用起来。完成后用户能在 GUI 里完整体验「点运行 → 看执行 tab → 节点变色 → 日志刷新」。

**新增文件**：
```
frontend/src/types/execution.ts            # TS 类型
frontend/src/api/executions.ts             # Wails 绑定包装
frontend/src/features/execution/
  ExecutionStore.tsx                       # Context + reducer + 订阅 Wails 事件
  ExecutionList.tsx                        # HomePage 第三 tab 的列表
  ExecutionStatus.tsx                      # 状态图标组件（SVG）
  RunningBadge.tsx                         # 工作流编辑页的「运行中 ●● 2」卡片
frontend/src/pages/ExecutionDetailPage.tsx # 执行详情页（复用 WorkflowCanvas readOnly）
```

**修改文件**：
- `main.tsx`：包入 `ExecutionStoreProvider`
- `App.tsx`：新增路由 `/executions/:id` → `ExecutionDetailPage`；订阅 Wails 事件入口
- `pages/HomePage.tsx`：加第三 tab「执行」
- `pages/WorkflowCanvasPage.tsx`：顶栏加「▶ 运行」按钮 + RunningBadge
- `features/workflow/WorkflowCanvas.tsx`：加 `readOnly` prop（禁拖动、禁删、禁连）
- `features/workflow/nodes/BaseNode.tsx`：右上角状态角标 SVG
- `features/workflow/nodes/GenericNode.tsx`：从 data 读 nodeState 传给 BaseNode
- `features/workflow/NodeDetailPanel.tsx`：拆 Config / Logs / Info 三 tab
- `features/tabs/TabsContext.tsx`：TabKind 加 `'execution'`
- `features/tabs/TabBar.tsx`：execution tab 显示 ▶ + 状态色

**ExecutionStore 形状**：
```ts
interface ExecutionState {
  id: string;
  workflowID: string;
  snapshot: ExecutionSnapshot;
  status: 'running' | 'success' | 'failed' | 'terminated';
  startedAt: number;
  finishedAt?: number;
  nodeStates: Record<string, NodeState>;
  nodeLogs: Record<string, LogEntry[]>;
  variables: Record<string, unknown>;
  error?: string;
}

interface ExecutionStoreValue {
  executions: Map<string, ExecutionState>;
  getByWorkflow: (workflowID: string) => ExecutionState[];
}
```

**Wails 事件订阅**：
```tsx
// App.tsx
useEffect(() => {
  const offs = [
    EventsOn('execution:started', (p) => dispatch({ type: 'started', ...p })),
    EventsOn('execution:node', (p) => dispatch({ type: 'node', ...p })),
    EventsOn('execution:log', (p) => dispatch({ type: 'log', ...p })),
    EventsOn('execution:variable', (p) => dispatch({ type: 'variable', ...p })),
    EventsOn('execution:status', (p) => dispatch({ type: 'status', ...p })),
    EventsOn('execution:finished', (p) => dispatch({ type: 'finished', ...p })),
  ];
  return () => offs.forEach((off) => off());
}, []);
```

**验收**：
- 点「▶ 运行」按钮后顶栏自动添加 execution tab，并切到该 tab
- 执行详情页画布上 print 节点先变蓝（执行中）后变绿（成功）
- 右侧详情面板 Logs tab 显示 `[INFO] Hello World`
- 顶栏 RunningBadge 在运行期间显示 `运行中 1`，运行完消失
- HomePage 第三 tab 列出运行过的执行
- 点击执行列表的「查看」/「×」按钮分别能打开 detail 和移除内存记录

---

### Phase 4：Stack Frame + 集合调用

**目标**：工作流里调用集合能正确执行，集合内变量与工作流变量隔离。

**修改文件**：
- `internal/engine/runtime.go`：添加 frame 栈管理 + push/pop
- `internal/engine/engine.go`：遇到 `assemble:<id>` 节点时 push frame 同步执行集合内部
- `internal/engine/snapshot.go`：递归收集所有引用的集合到 ExecutionSnapshot
- `internal/nodes/assemble_start/`：实装 Execute（空操作，作为集合 frame 的起点标记）
- `internal/nodes/assemble_end/`：实装 Execute（收集 input 端口值到 frame.Returns）
- `internal/nodes/assemble_param/`：实装 Execute（从 frame.Params 取值）

**集合调用执行流程**：
```go
// engine.go 处理 assemble:<id> 节点
func (r *Runtime) execAssembleCall(node NodeInstance) (Outputs, error) {
    assembleID := strings.TrimPrefix(node.TypeID, "assemble:")
    asm := r.snapshot.Assembles[assembleID]

    // 1. 计算调用方传入的参数
    params := map[string]any{}
    for _, p := range asm.Params {
        v, _ := r.evalInput(node.InstanceID, "param_"+p.Name)
        params[p.Name] = v
    }

    // 2. push frame
    frame := &Frame{
        AssembleID: assembleID,
        Variables:  initVariables(asm.Variables),
        Params:     params,
        Returns:    map[string]any{},
        Parent:     r.currentFrame(),
    }
    r.pushFrame(frame)
    defer r.popFrame()

    // 3. 从 assemble_start 开始执行集合内部
    startID := findStartNode(asm.Nodes)
    if err := r.executeFlowIn(asm, startID); err != nil {
        return nil, err
    }

    // 4. 把 frame.Returns 转成本节点的 outputs
    out := Outputs{}
    for k, v := range frame.Returns {
        out["return_"+k] = v
    }
    return out, nil
}
```

**验收**：
- 创建集合 A，参数 host(String)，返回 result(String)，内部用 print 输出 host
- 工作流内调用 A，给 host 传 "hello"
- 运行后日志包含 "hello"
- A 的 Variables（如果有）跟工作流变量互不影响
- 嵌套调用（A 调用 B）也正常

---

### Phase 5：并发 + 线程节点

**目标**：parallel 真并发执行，thread 后台独立线程，主流不等。

**修改文件**：
- `internal/nodes/parallel/parallel.go`：实装 Execute（goroutine + sync.WaitGroup）
- `internal/nodes/thread/thread.go`：实装 Execute（spawn goroutine，主流立即返回）
- `internal/engine/runtime.go`：Variables map 加 sync.RWMutex（并发写保护）
- `internal/engine/runtime.go`：追踪所有活跃 goroutine（WaitGroup），终态判定改为「全部归零」

**Parallel TypeDef**：
- ConfigSchema 加 `branch_count`（int，2~10）
- ExecutionMode 是 flow，但执行时 spawn N 个 goroutine
- TypeDef 的 OutputPorts 由 branch_count 动态生成 N 个 + 1 个 exec_out_done
  - 由于 ConfigSchema 改变导致端口数变化，前端 GenericNode 需要响应 config 变化重新计算 ports
  - 或者：固定 N=8，UI 只显示已连接的端口（更简单）—— **本阶段采纳固定 N=8 方案**

**Thread TypeDef**：
- 2 个 exec_out：`exec_out_continue` + `exec_out_thread`

**并发 Execute 骨架**：
```go
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
    rt := ctx.(*engine.ExecContextImpl).Runtime()  // 引擎内部访问
    nodeID := ctx.NodeID()
    var wg sync.WaitGroup
    var firstErr error
    var mu sync.Mutex
    for i := 1; i <= 8; i++ {
        portID := fmt.Sprintf("exec_out_%d", i)
        next := rt.FindNextExec(nodeID, portID)
        if next == "" { continue }
        wg.Add(1)
        go func() {
            defer wg.Done()
            if err := rt.ExecuteFlow(next); err != nil {
                mu.Lock(); if firstErr == nil { firstErr = err }; mu.Unlock()
            }
        }()
    }
    wg.Wait()
    return nil, firstErr  // 引擎接着走 exec_out_done
}
```

**注**：上面例子里 `ctx.(*engine.ExecContextImpl)` 这种向下转型不优雅。改进：ExecContext 接口加 `SpawnExec(portID) error` 和 `WaitAll()` 方法暴露给 flow_control 节点。具体在 Phase 5 落地时再决定接口形态。

**验收**：
- 用 parallel 同时跑 3 条分支（每条一个 print），日志能看到三条几乎同时产生
- 用 thread spawn 一个无限 print 循环节点（后续实装一个 loop 节点配合测试），主流能正常走到尽头并标记为 success
- 停止工作流时 thread 内的循环也被 cancel
- 并发 var_set 同一变量不 panic（mutex 生效）

---

### Phase 6：system_update + system_over

**目标**：周期触发 + 终止钩子。

**新增文件**：
- `internal/engine/scheduler.go`：interval / cron 定时器管理

**修改文件**：
- `internal/nodes/system_update/system_update.go`：实装 Execute（空操作，作为 update 阶段流起点）
- `internal/nodes/system_over/system_over.go`：实装 Execute（同上）
- `internal/engine/engine.go`：Run 时识别 update 节点的 config，注册 ticker / cron 任务；Stop 时先触发 over 流再释放资源

**调度逻辑**：
- `delta_type=interval`：用 `time.Ticker`
- `delta_type=cron`：用 `github.com/robfig/cron/v3`
- `delta_type=manual`：仅手动触发（暂不暴露 UI）
- 每次 tick 启动一次新的 executeFlow（从 system_update 节点开始）
- 多个 tick 触发是否要等上次跑完？**等**（避免重叠堆积）

**验收**：
- 工作流配 system_update + print，interval=2s
- 运行后每 2 秒日志多一条 print 输出
- 点「停止」后停止 ticker，然后触发 system_over 那条流（如果配了 print），最后整个 execution 标记 terminated

---

### Phase 7：执行记录持久化

**目标**：重启 OpsEngine 后历史执行仍可查看。

**新增文件**：
- `internal/store/execution_store.go`：JSON 文件读写

**修改文件**：
- `internal/engine/engine.go`：终态执行调 `executionStore.Save`；启动时调 `List` 加载历史
- `app.go`：`ListExecutions` 合并内存 + 持久化记录
- `frontend/src/features/execution/ExecutionList.tsx`：列表加载历史
- `frontend/src/pages/ExecutionDetailPage.tsx`：详情页从 `GetExecution` 拉数据（不再仅依赖内存 store）

**存储路径**：`data/executions/<execution_id>.json`

**注意点**：
- 仅终态（success / failed / terminated）写盘，running 不写
- snapshot 较大，可能 1MB 量级；列表用 `ExecutionSummary`（剥离 snapshot 和 logs）减小传输量
- 删除记录时删除对应文件

**验收**：
- 跑完一次执行 → 退出 wails dev → 重启 → 在 HomePage 的执行 tab 仍能看到 → 进详情仍能看节点状态和日志

---

## 7. 验收清单总览

| Phase | 关键里程碑 |
|-------|------------|
| 0 | 编译通过，节点列表无变化，添加 parallel/thread 节点 UI 可见 |
| 1 | 拖第二条 exec_out 被拒绝；含 2 个 system_ready 的工作流保存失败 |
| 2 | 后端日志能看到 Hello World 事件流推送 |
| 3 | GUI 端从「点运行」到「看到日志」全流程通畅 |
| 4 | 工作流调用集合，参数/返回值正确，变量隔离 |
| 5 | parallel 真并发，thread 后台跑且不阻塞主流 |
| 6 | interval 触发 update，停止时触发 over |
| 7 | 重启后历史执行可恢复查看 |

---

## 8. 风险与开放问题

- **Wails 事件高频性能**：极端情况下（1000 条/秒日志）前端可能卡。如出现，引擎侧加 100ms 批量缓冲，事件改为 `execution:logs`（数组形式）
- **节点 config schema 默认值类型**：`Default any` 在 TOML/JSON 跨语言序列化时需要约定（建议都用 string 表示，节点 Execute 自己 parse）
- **并发节点端口数固定 8 vs 动态**：本计划暂用固定 8 个，UI 隐藏空端口；如果体验不好再做动态
- **集合 hot-reload**：用户在 frontend 改集合时，后端的 GetNodeTypes 已经 invalidate node-types 缓存（✓ 已实现）。运行中的 ExecutionRecord 仍用自己的 snapshot，不受影响

---

## 9. 下一步

按 Phase 0 → 1 → 2 → 3 顺序推进。每个 Phase 完成后做以下事情再进入下一个：

1. 编译 + 类型检查通过（`go build`、`tsc --noEmit`）
2. 启动应用人工验收对应阶段的 acceptance
3. 复盘是否暴露出本计划没考虑到的问题，更新本文档
