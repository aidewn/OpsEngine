# OpsEngine 技术设计文档 v0.1（Go 版）

> 单机可视化运维工作流引擎

---

## 目录

1. [项目概述](#1-项目概述)
2. [技术选型](#2-技术选型)
3. [项目结构](#3-项目结构)
4. [核心概念](#4-核心概念)
5. [工作流机制](#5-工作流机制)
6. [节点开发框架](#6-节点开发框架)
7. [Agent 模式](#7-agent-模式)
8. [子工作流机制](#8-子工作流机制)
9. [终止机制](#9-终止机制)
10. [存储设计](#10-存储设计)
11. [API 设计](#11-api-设计)
12. [内置节点目录](#12-内置节点目录)
13. [工作流配置文件格式](#13-工作流配置文件格式)
14. [开发阶段规划](#14-开发阶段规划)

---

## 1. 项目概述

OpsEngine 是一个**单机可视化拖拽连线运维工具**。核心思想是将运维操作抽象为**节点卡片**，通过**连线**描述数据依赖，组合成**工作流**执行。

### 1.1 设计原则

- **后端优先**：后端不依赖前端独立运行，前端只是后端状态的展示镜子
- **自治工作流**：每个工作流是自治的生命体，外部只能激活，不能干预内部
- **框架约束**：节点开发者被约束在统一框架内，只实现业务逻辑
- **零数据库依赖**：纯文件存储，TOML 定义 + JSON 执行历史 + 文本日志

### 1.2 技术栈

| 层次 | 技术选型 |
|------|---------|
| 后端服务 | Go 1.22+ |
| HTTP 框架 | Gin |
| WebSocket | gorilla/websocket |
| SSH 客户端 | golang.org/x/crypto/ssh |
| 配置解析 | BurntSushi/toml |
| 日志 | zap |
| UUID | google/uuid |
| 存储 | 纯文件（无数据库） |
| 前端（后期） | React + TypeScript + React Flow |

---

## 2. 技术选型

### 2.1 为什么选 Go

| 维度 | 说明 |
|------|------|
| goroutine + context | 天然适合工作流并发调度，终止机制用 context.Cancel 即可 |
| 交叉编译 | 一个环境变量搞定，无需额外工具链 |
| SSH 官方库 | golang.org/x/crypto/ssh 稳定成熟 |
| 开发效率 | 比 Rust 更快，错误提示友好 |
| 运维生态 | 大量成熟的运维工具用 Go 编写，社区资源丰富 |

### 2.2 依赖清单

```
github.com/gin-gonic/gin              HTTP 框架
github.com/gorilla/websocket          WebSocket
golang.org/x/crypto                   SSH 客户端
github.com/BurntSushi/toml            TOML 配置解析
github.com/google/uuid                UUID 生成
go.uber.org/zap                       结构化日志
github.com/stretchr/testify           单元测试
```

---

## 3. 项目结构

```
ops-engine/
├── cmd/
│   ├── ops-engine/
│   │   └── main.go              后端服务入口
│   └── ops-agent/
│       └── main.go              Agent 可执行文件入口
│
├── internal/
│   ├── core/                    共享类型定义（无 IO 依赖）
│   │   ├── types.go             工作流、节点、Handle 基础类型
│   │   ├── workflow.go          工作流定义结构
│   │   ├── node.go              节点实例、状态枚举
│   │   ├── handle.go            Handle 类型定义
│   │   ├── edge.go              连线定义
│   │   └── agent.go             Agent 任务、回调结构
│   │
│   ├── framework/               节点开发框架
│   │   ├── interfaces.go        NodeBase / CheckableNode / RemoteCmdNode / AgentNode
│   │   ├── runner.go            框架统一编排入口
│   │   └── checker.go           CheckItem 执行器
│   │
│   ├── nodes/                   内置节点实现
│   │   ├── connection/
│   │   │   └── linux_ssh.go
│   │   ├── nginx/
│   │   │   ├── nginx_linux.go
│   │   │   ├── nginx_docker.go
│   │   │   └── nginx_k8s.go
│   │   └── flow/
│   │       ├── break_guard.go
│   │       └── break_signal.go
│   │
│   ├── registry/
│   │   └── registry.go          节点类型注册表
│   │
│   ├── runtime/                 工作流运行时
│   │   ├── executor.go          工作流执行引擎
│   │   ├── handle_store.go      Handle 内存存储
│   │   ├── loop_controller.go   Break 令牌、Delta 计时
│   │   └── log_collector.go     日志暂存与批量写入
│   │
│   ├── store/                   持久化层（纯文件）
│   │   ├── workflow_store.go    TOML 工作流读写
│   │   └── execution_store.go   执行历史 JSON + 日志文件
│   │
│   └── api/
│       ├── rest.go              REST 接口路由
│       ├── handlers.go          各接口处理函数
│       └── ws.go                WebSocket 状态推送
│
├── agent/                       Agent 内部实现
│   ├── interfaces.go            AgentTask interface
│   ├── logger.go                任务日志收集器
│   ├── callback.go              HTTP 回调逻辑
│   └── tasks/
│       ├── nginx_install.go
│       └── nginx_check.go
│
├── agents/                      编译好的 Agent 二进制
│   ├── linux_amd64/
│   │   └── ops-agent
│   └── linux_arm64/
│       └── ops-agent
│
├── data/                        运行时数据目录
│   ├── workflows/               工作流 TOML 定义
│   ├── executions/              执行历史 JSON
│   └── logs/                    节点执行日志
│
├── go.mod
├── go.sum
└── Makefile                     常用构建命令
```

---

## 4. 核心概念

### 4.1 节点（Node）

运维操作的最小单元，分为三个阶段：

| 阶段 | 触发时机 | 说明 |
|------|---------|------|
| 配置阶段 | 用户编辑时 | 填写参数，由 `ConfigSchema` 驱动前端表单 |
| 检查阶段 | 用户点击检查按钮 | 验证前置条件，每个检查项独立展示 |
| 执行阶段 | 工作流运行时 | 实际执行操作，产出 Handle |

#### 节点状态机

```
Idle
  └─► Configuring（用户开始填写配置）
        └─► Checking（点击检查）
              ├─► CheckFailed（检查未通过）
              └─► Ready（可产出 handle 连线）
                    └─► Executing（工作流运行时）
                          ├─► Success
                          ├─► Failed（触发全局终止）
                          └─► Skipped（上游节点失败）
```

### 4.2 Handle

节点执行后产出的**连接句柄**，在连线上流动的数据载体。

**关键设计：**
- Handle 实际值只存在于**后端内存**
- 前端只持有 `handle_id`（UUID），不接触敏感数据
- Handle 生命周期与工作流运行时绑定，终止后全部释放

**Handle 类型：**

```go
// Handle 值的 interface，所有具体 Handle 类型实现它
type HandleValue interface {
    HandleType() PortType
}

// SSH 连接 Handle
type SshHandle struct {
    Client   *ssh.Client
    Host     string
    Port     int
    Username string
}
func (s *SshHandle) HandleType() PortType { return PortTypeLinuxSsh }

// Nginx 实例 Handle
type NginxHandle struct {
    Version    string
    ConfigPath string
}
func (n *NginxHandle) HandleType() PortType { return PortTypeNginxInstance }
```

### 4.3 端口类型系统

连线只能连接**类型匹配**的端口：

```go
type PortType string

const (
    PortTypeLinuxSsh      PortType = "LinuxSshConnection"
    PortTypeDockerContext  PortType = "DockerContext"
    PortTypeK8sContext     PortType = "K8sContext"
    PortTypeNginxInstance  PortType = "NginxInstance"
    PortTypeFlowSignal     PortType = "FlowSignal"
)
```

---

## 5. 工作流机制

### 5.1 事件源节点（Event Source Nodes）

工作流的生命周期由三种**事件源节点**驱动。它们是注册到 Registry 的**普通节点**，可以像任何节点一样添加、连接、删除；引擎在特定生命周期时刻"触发"它们，使其向下游输出 `FlowSignal`。

| TypeID | 触发时机 | 触发次数 | 输出端口 |
|--------|---------|---------|---------|
| `system_ready` | 工作流启动时 | 一次 | `signal: FlowSignal` |
| `system_update` | 自身 delta 周期 | 循环 | `signal: FlowSignal` |
| `system_over` | 工作流终止时 | 一次 | `signal: FlowSignal` |

**设计思想（类比 Unreal Blueprint 的 Event 节点）：**
- 没有"系统阶段"的抽象，只有"事件源节点"
- 业务节点通过连线接收 `FlowSignal` 来被触发执行
- 哪些节点会执行、何时执行，完全由连线拓扑决定
- 节点本身不携带 `stage` 字段——它的"阶段归属"由"被哪个事件源可达"自然导出

**关键特性：**
- 同一工作流可以有**多个 `system_update` 节点**，各自配置不同 delta，相当于 Blueprint 多个 Tick 速率
- 工作流可以**没有任何事件源节点**——但这样的工作流没有任何可执行内容
- 用户**可以删除**全部或部分事件源节点，前端在新建工作流时**作为便利会自动放置一组**，并非强制

#### `system_update` 节点配置

```toml
[[nodes]]
id   = "tick_60s"
type = "system_update"

[nodes.config]
delta_type    = "interval"      # interval | cron | manual
delta_seconds = 60
# cron_expr  = "*/5 * * * *"   # delta_type = cron 时生效
# manual                       # 只触发一次，等同于一次性脚本
```

每个 `system_update` 节点独立维护自己的循环节奏与 Break 令牌（通过节点 ID 隔离）。

#### 便利约定：默认创建 3 个事件源节点

为减少用户每次新建工作流的重复操作，前端在调用 `POST /api/workflows` 创建空工作流后，会**自动追加** `system_ready` / `system_update` / `system_over` 三个节点并保存。这只是前端便利功能，后端没有任何特殊处理。

### 5.2 执行有效范围

每次事件源节点被触发时，从该节点出发沿连线方向做可达性分析，只执行可达子图内、且状态为 Ready 的业务节点：

```
某事件源节点被触发
  └── 沿其 signal 输出连线广度遍历
        ├── 命中业务节点：状态为 Ready → 执行
        ├── 孤立节点：永远不会被触发 → 忽略
        └── 上游失败的节点 → 标记 Skipped
```

注意：同一业务节点可以同时被 `system_ready` 和 `system_update` 上溯到——这是合法的（初始化时执行一次、循环时也执行）。是否重复触发由引擎根据"被哪个事件触发"自然处理，节点本身不需要任何"stage"声明。

### 5.3 工作流状态

```go
type WorkflowStatus string

const (
    WorkflowStatusIdle       WorkflowStatus = "Idle"
    WorkflowStatusRunning    WorkflowStatus = "Running"
    WorkflowStatusTerminated WorkflowStatus = "Terminated"
)

// 当前生命周期阶段（仅用于 UI 观测）
// 由引擎根据正在处理哪类事件源派生
type WorkflowPhase string

const (
    WorkflowPhaseStarting    WorkflowPhase = "Starting"    // 正在触发 system_ready 节点
    WorkflowPhaseRunning     WorkflowPhase = "Running"     // 至少一个 system_update 正在循环
    WorkflowPhaseTerminating WorkflowPhase = "Terminating" // 正在触发 system_over 节点
)
```

### 5.4 Break 循环控制机制

`system_update` 节点循环触发时的**令牌控制机制**，防止上一轮未完成就开始下一轮。

#### Break Guard 节点
- 通常放在 `system_update` 下游链路**开头**
- 首轮直接放行，后续等待上一轮令牌
- 配置 `bound_event`：指定它绑定到哪个 `system_update` 节点的循环（多 update 节点场景下用于隔离令牌环）

#### Break Signal 节点
- 放在循环链路**末尾**
- 向上游对应的 Break Guard 发出令牌
- 可配置条件：`only_on_success` 或 `always`

#### Go 实现：channel 天然适合令牌机制

```go
type LoopController struct {
    // 缓冲为 1，防止重复发令牌
    tokenCh    chan struct{}
    LoopCount  int
}

func NewLoopController() *LoopController {
    return &LoopController{
        tokenCh: make(chan struct{}, 1),
    }
}

// Break Signal 调用
func (lc *LoopController) EmitToken() {
    select {
    case lc.tokenCh <- struct{}{}:
    default: // 已有令牌，不重复发
    }
}

// Break Guard 调用
func (lc *LoopController) WaitForToken(ctx context.Context) bool {
    if lc.LoopCount == 0 {
        return true // 首轮直接放行
    }
    select {
    case <-lc.tokenCh:
        return true
    case <-ctx.Done():
        return false // 工作流已终止
    }
}
```

---

## 6. 节点开发框架

框架的核心价值：**开发者只关心业务逻辑，框架承担生命周期编排、错误处理、日志收集、状态推送等通用逻辑。**

### 6.1 框架职责边界

| 框架负责 | 开发者负责 |
|---------|----------|
| 生命周期编排 | 节点类型描述（`Define()`） |
| 错误处理与状态流转 | 配置格式校验（`Validate()`） |
| 心跳监控（Agent 模式） | 检查项定义（`CheckItems()`） |
| 日志收集与批量写入 | 核心执行逻辑 |
| 超时控制 | Handle 产出 |
| cleanup 保证执行 | |
| WebSocket 状态推送 | |

### 6.2 Interface 层次结构

```go
// ── 所有节点必须实现 ──────────────────────────────────────
type NodeBase interface {
    Define() NodeTypeDef
    Validate(config map[string]any) ValidateResult
}

// ── 带检查项的节点 ─────────────────────────────────────────
type CheckableNode interface {
    NodeBase
    CheckItems(config map[string]any, inputs HandleMap) []CheckItem
}

// ── 远程指令执行模式 ──────────────────────────────────────
type RemoteCmdNode interface {
    CheckableNode
    ExecuteCmds(config map[string]any, inputs HandleMap) []RemoteCmd
    OnOutput(config map[string]any, outputs []CmdOutput) (HandleMap, error)
}

// ── Agent 上传执行模式 ────────────────────────────────────
type AgentNode interface {
    CheckableNode
    Prepare(config map[string]any) []UploadFile
    TaskConfig(config map[string]any) map[string]any
    OnCallback(result AgentCallback) (HandleMap, error)
}

// ── 流控节点 ──────────────────────────────────────────────
type FlowNode interface {
    NodeBase
    ExecuteFlow(ctx context.Context, config map[string]any, execCtx *ExecContext) (HandleMap, error)
}
```

### 6.3 核心数据结构

```go
// HandleMap：节点间传递 Handle 的容器，key 是端口 ID
type HandleMap map[string]HandleValue

// 节点类型定义（注册时确定，驱动前端渲染）
type NodeTypeDef struct {
    TypeID        string        `json:"type_id"`
    DisplayName   string        `json:"display_name"`
    Category      string        `json:"category"`
    Icon          string        `json:"icon"`
    Description   string        `json:"description"`
    InputPorts    []PortDef     `json:"input_ports"`
    OutputPorts   []PortDef     `json:"output_ports"`
    ConfigSchema  []FieldSchema `json:"config_schema"`
    ExecutionMode string        `json:"execution_mode"` // remote_cmd | agent | flow
}

// 端口定义
type PortDef struct {
    ID       string   `json:"id"`
    Label    string   `json:"label"`
    PortType PortType `json:"port_type"`
    Required bool     `json:"required"`
}

// 配置字段 Schema
type FieldSchema struct {
    Type        string   `json:"type"` // text|password|number|select|toggle
    ID          string   `json:"id"`
    Label       string   `json:"label"`
    Placeholder string   `json:"placeholder,omitempty"`
    Required    bool     `json:"required,omitempty"`
    Min         *int64   `json:"min,omitempty"`
    Max         *int64   `json:"max,omitempty"`
    Default     any      `json:"default,omitempty"`
    Options     []string `json:"options,omitempty"`
}

// 配置校验结果
type ValidateResult struct {
    Passed bool
    Errors []FieldError
}

type FieldError struct {
    Field   string
    Message string
}

// 检查项
type CheckItem struct {
    Label    string
    Required bool
    Action   func() CheckItemResult
}

type CheckItemResult struct {
    Passed bool
    Detail string
}

// 远程命令
type RemoteCmd struct {
    Cmd         string
    Description string
    TimeoutSecs int
}

type CmdOutput struct {
    Cmd      string
    Stdout   string
    Stderr   string
    ExitCode int
}

// 上传文件描述（AgentNode 用）
type UploadFile struct {
    Type       string // "agent_binary" | "task_config" | "custom"
    LocalPath  string // custom 类型时使用
    RemotePath string
    Content    []byte
}

// 执行上下文
type ExecContext struct {
    ExecutionID string
    NodeID      string
    EngineAddr  string   // Agent 回调地址
    Logger      *Logger
    Ctx         context.Context
}
```

### 6.4 框架统一编排流程

```
NodeRunner.Run(node, config, inputs, execCtx)
│
├── 1. node.Validate(config)
│         失败 → 节点状态 CheckFailed
│
├── 2. node.CheckItems() 逐项执行
│         实时推送每项结果到 WebSocket
│         有必要项失败 → 节点状态 CheckFailed
│
├── 3. 执行阶段（根据节点类型断言分支）
│   │
│   ├── RemoteCmdNode
│   │     ExecuteCmds() → SSH 逐条执行 → OnOutput()
│   │
│   ├── AgentNode
│   │     Prepare() → SSH 上传文件 → 启动 Agent
│   │     goroutine 心跳监控（SSH 检查 pid）
│   │     等待 /api/agent/callback 回调
│   │     OnCallback() → HandleMap
│   │
│   └── FlowNode
│         ExecuteFlow() → 直接返回
│
├── 4. defer cleanup（Go 的 defer 保证无论成功失败都执行）
│         Agent 模式：SSH 删除远程残留文件
│
├── 5. 日志批量写入文件
│
└── 6. 推送最终节点状态 WebSocket 事件
```

### 6.5 新节点开发示例

以 `nginx_with_linux` 为例，开发者实现约 **80 行**：

```go
type NginxWithLinuxNode struct{}

// ── NodeBase ──────────────────────────────────────────────
func (n *NginxWithLinuxNode) Define() NodeTypeDef {
    return NodeTypeDef{
        TypeID:      "nginx_with_linux",
        DisplayName: "Nginx（Linux）",
        Category:    "部署",
        InputPorts: []PortDef{
            {ID: "server", PortType: PortTypeLinuxSsh, Required: true},
        },
        OutputPorts: []PortDef{
            {ID: "nginx_instance", PortType: PortTypeNginxInstance},
        },
        ConfigSchema: []FieldSchema{
            {Type: "select", ID: "version", Label: "Nginx 版本",
             Options: []string{"latest", "1.26", "1.24"}},
            {Type: "number", ID: "http_port", Label: "HTTP 端口", Default: 80},
            {Type: "toggle", ID: "start_on_install", Label: "安装后自动启动", Default: true},
        },
        ExecutionMode: "agent",
    }
}

func (n *NginxWithLinuxNode) Validate(config map[string]any) ValidateResult {
    port, _ := config["http_port"].(float64)
    if port <= 0 || port > 65535 {
        return ValidateResult{Passed: false, Errors: []FieldError{
            {Field: "http_port", Message: "端口范围必须在 1-65535 之间"},
        }}
    }
    return ValidateResult{Passed: true}
}

// ── CheckableNode ─────────────────────────────────────────
func (n *NginxWithLinuxNode) CheckItems(config map[string]any, inputs HandleMap) []CheckItem {
    port, _ := config["http_port"].(float64)
    return []CheckItem{
        {Label: "SSH 连接有效", Required: true, Action: func() CheckItemResult {
            // 通过 inputs["server"] 验证 SSH 连接
            return CheckItemResult{Passed: true}
        }},
        {Label: fmt.Sprintf("端口 %d 未占用", int(port)), Required: true, Action: func() CheckItemResult {
            // SSH 执行 ss -tlnp
            return CheckItemResult{Passed: true}
        }},
    }
}

// ── AgentNode ─────────────────────────────────────────────
func (n *NginxWithLinuxNode) Prepare(config map[string]any) []UploadFile {
    return []UploadFile{
        {Type: "agent_binary"},
        {Type: "task_config"},
    }
}

func (n *NginxWithLinuxNode) TaskConfig(config map[string]any) map[string]any {
    return map[string]any{
        "task_type": "nginx_install",
        "version":   config["version"],
        "http_port": config["http_port"],
        "start":     config["start_on_install"],
    }
}

func (n *NginxWithLinuxNode) OnCallback(result AgentCallback) (HandleMap, error) {
    return HandleMap{
        "nginx_instance": &NginxHandle{
            Version:    result.Output["nginx_version"].(string),
            ConfigPath: "/etc/nginx/nginx.conf",
        },
    }, nil
}
```

**注册一行搞定：**

```go
func BuildRegistry() *Registry {
    r := NewRegistry()
    r.Register(&connection.LinuxSshNode{})
    r.Register(&nginx.NginxWithLinuxNode{})  // ← 新节点
    r.Register(&flow.BreakGuardNode{})
    r.Register(&flow.BreakSignalNode{})
    return r
}
```

---

## 7. Agent 模式

### 7.1 Agent 完整生命周期

```
OpsEngine 侧                            远程服务器侧
────────────────────────────────────────────────────────

① 节点执行，生成 task_id

② SSH 执行 uname -m 检测架构

③ SSH 上传文件
   scp agents/linux_amd64/ops-agent → /tmp/ops-agent-<task_id>
   scp task.json → /tmp/task-<task_id>.json
   chmod +x /tmp/ops-agent-<task_id>

                                     ④ SSH 启动 Agent
                                        nohup /tmp/ops-agent-<task_id> \
                                          --task /tmp/task-<task_id>.json \
                                          --callback http://<engine>/api/agent/callback \
                                          --task-id <task_id> &
                                        echo $! > /tmp/ops-agent-<task_id>.pid

⑤ SSH 断开，节点进入等待状态

⑥ goroutine 心跳循环（每30秒）
   SSH cat /tmp/ops-agent-<task_id>.pid
   ├── 文件存在 → 继续等待
   └── 文件不存在
       等待 grace_period（10s）
       ├── 收到回调 → 正常处理
       └── 超时 → 节点 Failed

                                     ⑦ Agent 独立执行任务

                                     ⑧ 任务完成
                                        POST /api/agent/callback
                                        { task_id, status, log, output }

⑨ OpsEngine 收到回调
   channel 通知等待中的 goroutine
   调用 OnCallback()，Handle 存入 HandleStore

⑩ defer cleanup
   SSH 删除 /tmp/ops-agent-<task_id>*
```

### 7.2 回调等待：channel 实现

```go
// 等待 Agent 回调的机制
// AgentCallbackHub 管理所有等待中的 task

type AgentCallbackHub struct {
    mu       sync.Mutex
    waiters  map[string]chan AgentCallback // key: task_id
}

// 节点注册等待
func (h *AgentCallbackHub) Wait(taskID string, ctx context.Context) (AgentCallback, error) {
    ch := make(chan AgentCallback, 1)
    h.mu.Lock()
    h.waiters[taskID] = ch
    h.mu.Unlock()

    defer func() {
        h.mu.Lock()
        delete(h.waiters, taskID)
        h.mu.Unlock()
    }()

    select {
    case result := <-ch:
        return result, nil
    case <-ctx.Done():
        return AgentCallback{}, fmt.Errorf("等待 Agent 回调超时或工作流已终止")
    }
}

// /api/agent/callback 接口调用
func (h *AgentCallbackHub) Notify(callback AgentCallback) {
    h.mu.Lock()
    ch, ok := h.waiters[callback.TaskID]
    h.mu.Unlock()
    if ok {
        ch <- callback
    }
}
```

### 7.3 交叉编译（Go 的优势）

```bash
# 无需安装任何额外工具链
GOOS=linux GOARCH=amd64 go build -o agents/linux_amd64/ops-agent ./cmd/ops-agent
GOOS=linux GOARCH=arm64 go build -o agents/linux_arm64/ops-agent ./cmd/ops-agent
```

### 7.4 Agent 回调接口

```
POST /api/agent/callback

{
  "task_id":   "uuid-xxx",
  "status":    "success",        // success | failed
  "error_msg": null,
  "log":       "完整执行日志",
  "output": {
    "nginx_version": "1.26.1",
    "config_path":   "/etc/nginx/nginx.conf"
  }
}

响应：{ "received": true }
```

---

## 8. 子工作流机制

### 8.1 自我管理原则

| 运行身份 | 启动方式 | 说明 |
|---------|---------|------|
| 独立工作流 | 用户手动启动 | 自己管理完整生命周期 |
| 子工作流 | 父工作流显式激活 | 必须被激活才能运行 |

### 8.2 激活状态机

```go
type ActivationStatus string

const (
    ActivationDormant     ActivationStatus = "Dormant"
    ActivationActivating  ActivationStatus = "Activating"
    ActivationActivated   ActivationStatus = "Activated"
    ActivationRunningOver ActivationStatus = "RunningOver"
)
```

```
Dormant
  └─► Activating（父工作流激活，执行子 Ready）
        ├─► Ready 失败 → Dormant（保持）
        └─► Ready 成功 → Activated（Update 循环运行中）
              ├─► 父更新参数 → 继续 Update，使用新参数
              └─► 终止信号 / Update 失败
                    └─► RunningOver（执行 Over）
                          └─► Over 完成 → Dormant（可再次激活）
```

**关键规则：**
- 已激活状态收到激活请求 → **只更新参数**，不重新执行 Ready
- Over 执行中收到激活请求 → **排队**，Over 完成后自动激活
- Ready 失败 → 保持 Dormant，父工作流可配置重试或跳过

### 8.3 参数注入

子工作流通过一个特殊的节点类型 `sub_workflow_call` 嵌入父工作流，激活时机由它的 `signal` 输入端口决定（连到哪个事件源就在何时激活）：

```toml
[[nodes]]
id   = "deploy_nginx_sub"
type = "sub_workflow_call"

[nodes.config]
workflow = "deploy_nginx"        # 引用的子工作流定义 ID

# 端口映射通过普通连线完成
# 子工作流的外部输入由 sub_workflow_call 的动态输入端口表达
# 子工作流的外部输出由 sub_workflow_call 的动态输出端口表达

[[edges]]
from = { node = "system_ready_1", port = "signal"   }
to   = { node = "deploy_nginx_sub", port = "trigger" }

[[edges]]
from = { node = "ssh_conn",         port = "ssh_conn" }
to   = { node = "deploy_nginx_sub", port = "server"   }
```

`sub_workflow_call` 节点根据所引用子工作流的"暴露接口"（在子工作流中标记为外部输入/输出的端口）动态生成自身的端口集合，前端据此渲染。

### 8.4 Over 级联顺序

终止时由内向外触发 Over：

```
最深层子工作流 Over 完成
  └── 上层子工作流 Over 完成
        └── 主工作流 Over 完成
              └── 释放所有 Handle
```

---

## 9. 终止机制

### 9.1 触发条件（一票否决）

- 用户手动点击终止
- 任意节点 Failed（包括子工作流内部节点）
- 进程收到 SIGTERM

**不支持暂停/恢复，只能强制终止。**

### 9.2 Go 实现：context.Context

```go
// context.WithCancel 是 Go 标准的终止传播机制
// 调用 cancel() 后，所有持有这个 ctx 的 goroutine 都能感知到

type WorkflowExecutor struct {
    ctx    context.Context
    cancel context.CancelFunc
    // ...
}

func NewExecutor(def WorkflowDef) *WorkflowExecutor {
    ctx, cancel := context.WithCancel(context.Background())
    return &WorkflowExecutor{ctx: ctx, cancel: cancel}
}

// 任意位置触发终止
func (e *WorkflowExecutor) Terminate(reason string) {
    e.terminateReason = reason
    e.cancel() // 所有监听 ctx.Done() 的 goroutine 自动停止
}

// 节点执行时监听终止
func runNode(ctx context.Context, node NodeBase) error {
    select {
    case <-ctx.Done():
        return fmt.Errorf("工作流已终止")
    default:
        // 继续执行
    }
    // ...
}
```

### 9.3 终止传播顺序

```
① ctx.Cancel() 广播到所有 goroutine
② 等待当前执行中的节点完成（或超时）
③ 由内向外触发 Over 阶段
④ 释放所有 Handle（SSH 连接 Close）
⑤ 批量写入日志和执行记录
```

### 9.4 终止时触发 `system_over` 节点（defer）

```go
// Go 的 defer 保证无论成功失败都执行，类似 try-finally
func (e *WorkflowExecutor) Run() {
    defer e.fireSystemOverNodes() // 触发所有 system_over 节点的下游子图
    defer e.cleanup()             // 最后释放 Handle 等资源

    e.fireSystemReadyNodes()      // 触发所有 system_ready 节点的下游子图
    e.runUpdateLoops()            // 启动所有 system_update 节点的循环
}
```

`fireSystemOverNodes` 内部对所有 `system_over` 节点执行可达性分析，按拓扑序执行其下游子图。即使 Ready 阶段失败、Update 阶段被终止，Over 节点的下游子图也保证被触发（除非进程崩溃 panic）。

---

## 10. 存储设计

### 10.1 目录结构

```
data/
├── workflows/
│   ├── deploy_nginx.toml
│   └── init_server.toml
│
├── executions/
│   ├── exec_20250522_143001_abc.json
│   └── exec_20250522_150012_def.json
│
└── logs/
    └── exec_20250522_143001_abc/
        ├── ssh_conn.log
        └── install_nginx.log
```

### 10.2 执行历史 JSON

```json
{
  "id":               "exec_20250522_143001_abc",
  "workflow_id":      "deploy_nginx",
  "workflow_name":    "部署 Nginx",
  "status":           "success",
  "terminate_reason": null,
  "started_at":       "2025-05-22T14:30:01Z",
  "finished_at":      "2025-05-22T14:33:12Z",
  "nodes": [
    {
      "id":             "ssh_conn",
      "node_type":      "linux_ssh_connection",
      "triggered_by":   "system_ready_1",
      "status":         "success",
      "loop_count":     null,
      "error_msg":      null,
      "started_at":     "2025-05-22T14:30:02Z",
      "finished_at":    "2025-05-22T14:30:03Z"
    }
  ]
}
```

### 10.3 日志策略

- 节点执行中日志**暂存内存**
- 节点完成后**一次性写入** `.log` 文件
- Agent 模式：回调中的 `log` 字段直接写入文件

---

## 11. API 设计

### 11.1 REST 接口

```
节点类型
GET  /api/node-types                           所有节点类型定义

工作流定义
GET  /api/workflows                            列表
GET  /api/workflows/:id                        详情
POST /api/workflows                            创建（JSON body）
PUT  /api/workflows/:id                        更新
DELETE /api/workflows/:id                      删除

工作流控制
POST /api/workflows/:id/start                  启动
POST /api/workflows/:id/terminate              强制终止
POST /api/workflows/:id/activate               作为子工作流激活

执行历史
GET  /api/executions                           列表（时间倒序）
GET  /api/executions/:id                       详情
GET  /api/executions/:id/nodes/:node_id/log    节点日志

Agent 回调（内部接口）
POST /api/agent/callback                       Agent 执行完成回调
```

### 11.2 WebSocket 事件

```
WS /ws/executions/:id

事件类型（JSON）：

{ "event": "NodeStateChanged",      "node_id": "ssh_conn",   "old": "Checking",  "new": "Ready" }
{ "event": "CheckItemResult",       "node_id": "ssh_conn",   "label": "端口可达", "passed": true }
{ "event": "WorkflowPhaseChanged",  "phase": "Running" }
{ "event": "EventSourceFired",      "node_id": "tick_60s",   "type": "system_update", "loop_count": 3 }
{ "event": "SubWorkflowActivated",  "node_id": "deploy_nginx_sub" }
{ "event": "BreakTokenIssued",      "guard_node_id": "guard_1", "loop_count": 3 }
{ "event": "LoopSkipped",           "guard_node_id": "guard_1", "loop_count": 4, "reason": "no_break_token" }
{ "event": "WorkflowTerminated",    "status": "Failed",      "triggered_by": "install_nginx" }
```

### 11.3 节点执行子状态

```
Executing:preparing    上传 Agent 文件
Executing:launched     Agent 已启动，等待回调
Executing:watching     心跳监控中（第 N 次）
Executing:completing   收到回调，处理结果
```

---

## 12. 内置节点目录

### 12.1 连接类

#### `linux_ssh_connection`

```
执行模式：RemoteCmd
输入端口：无
输出端口：ssh_conn（LinuxSshConnection）

配置：host / port(22) / username / password

检查项：
  ✓ TCP 端口可达（5s 超时）
  ✓ SSH 握手成功
  ✓ 密码认证成功

执行：建立 SSH 连接，存入 HandleStore
```

### 12.2 部署类

#### `nginx_with_linux`

```
执行模式：Agent
输入端口：server（LinuxSshConnection，必须）
输出端口：nginx_instance（NginxInstance）

配置：version / http_port(80) / start_on_install(true)

检查项：
  ✓ SSH 连接有效
  ✓ 目标端口未占用
  ✓ 磁盘空间 > 200MB
  ✓ 检测包管理器（apt/yum）

Agent 任务：apt/yum install nginx
```

#### `nginx_with_docker`

```
执行模式：Agent
输入端口：server（LinuxSshConnection，必须）
输出端口：nginx_instance（NginxInstance）

配置：image_tag / container_name / host_http_port / volumes

检查项：
  ✓ Docker daemon 可达
  ✓ 端口未占用
  ✓ 镜像仓库可达

Agent 任务：docker pull → docker run
```

#### `nginx_with_k8s`

```
执行模式：Agent
输入端口：k8s_ctx（K8sContext，必须）
输出端口：nginx_instance（NginxInstance）

配置：namespace / replicas / image_tag / service_type

检查项：
  ✓ K8s 连接有效
  ✓ Namespace 存在
  ✓ 资源配额充足

Agent 任务：kubectl apply Deployment + Service
```

### 12.3 流控类

#### `break_guard`

```
执行模式：Flow
输入端口：trigger（FlowSignal，必须）
输出端口：signal（FlowSignal）

配置：bound_event（节点 ID，指定绑定到哪个 system_update 节点的循环令牌环）

行为：首轮直接放行，后续等待该 system_update 对应的令牌；
     令牌不存在且新一轮触发到达 → 跳过本轮，发出 LoopSkipped 事件
```

#### `break_signal`

```
执行模式：Flow
输入端口：trigger（FlowSignal，必须）

配置：
  bound_event（与对应 break_guard 一致，标识令牌环归属）
  condition（only_on_success | always）

行为：trigger 触发且 condition 满足 → 向对应令牌环发出令牌
```

### 12.4 系统事件源节点

#### `system_ready`

```
执行模式：Flow
输入端口：无
输出端口：signal（FlowSignal）

行为：工作流启动时由引擎触发一次，下游子图被同步执行
```

#### `system_update`

```
执行模式：Flow
输入端口：无
输出端口：signal（FlowSignal）

配置：
  delta_type（interval | cron | manual）
  delta_seconds（interval 模式下生效）
  cron_expr（cron 模式下生效）

行为：按 delta 周期循环触发，每次触发激活下游可达子图。
     与 break_guard / break_signal 配合实现令牌式循环控制
```

#### `system_over`

```
执行模式：Flow
输入端口：无
输出端口：signal（FlowSignal）

行为：工作流终止时（用户终止 / 节点 Failed / SIGTERM）由引擎触发一次，
     用于优雅关闭、资源回收。即使前面阶段失败也保证执行
```

---

## 13. 工作流配置文件格式

工作流文件中所有节点（包括事件源节点）都是平等的 `[[nodes]]` 条目，没有 `stage` 字段。生命周期阶段完全由"哪个事件源连到哪些下游"自然导出。

```toml
[workflow]
id          = "deploy_nginx"
name        = "部署 Nginx"
description = "在 Linux 服务器上安装并持续监控 Nginx"

# ── 事件源节点 ──────────────────────────────────────────

[[nodes]]
id   = "ready_1"
type = "system_ready"
position = { x = 100, y = 100 }

[[nodes]]
id   = "tick_60s"
type = "system_update"
position = { x = 100, y = 400 }

[nodes.config]
delta_type    = "interval"
delta_seconds = 60

[[nodes]]
id   = "over_1"
type = "system_over"
position = { x = 100, y = 700 }

# ── 业务节点 ──────────────────────────────────────────

[[nodes]]
id   = "ssh_conn"
type = "linux_ssh_connection"
position = { x = 400, y = 100 }

[nodes.config]
host     = "192.168.1.10"
port     = 22
username = "root"
password = "${ENV:SERVER_PASSWORD}"

[[nodes]]
id   = "install_nginx"
type = "nginx_with_linux"
position = { x = 700, y = 100 }

[nodes.config]
version          = "1.26"
http_port        = 80
start_on_install = true

[[nodes]]
id   = "guard_1"
type = "break_guard"
position = { x = 400, y = 400 }

[nodes.config]
bound_event = "tick_60s"

[[nodes]]
id   = "check_nginx"
type = "nginx_with_linux"
position = { x = 700, y = 400 }

[nodes.config]
action = "check_status"

[[nodes]]
id   = "signal_1"
type = "break_signal"
position = { x = 1000, y = 400 }

[nodes.config]
bound_event = "tick_60s"
condition   = "only_on_success"

[[nodes]]
id   = "stop_nginx"
type = "nginx_with_linux"
position = { x = 400, y = 700 }

[nodes.config]
action = "stop"

# ── 连线 ────────────────────────────────────────────────

# Ready 链路：ready_1 → ssh_conn → install_nginx
[[edges]]
from = { node = "ready_1",       port = "signal"   }
to   = { node = "ssh_conn",      port = "trigger"  }

[[edges]]
from = { node = "ssh_conn",      port = "ssh_conn" }
to   = { node = "install_nginx", port = "server"   }

# Update 链路：tick_60s → guard_1 → check_nginx → signal_1
[[edges]]
from = { node = "tick_60s",      port = "signal"   }
to   = { node = "guard_1",       port = "trigger"  }

[[edges]]
from = { node = "guard_1",       port = "signal"   }
to   = { node = "check_nginx",   port = "trigger"  }

[[edges]]
from = { node = "ssh_conn",      port = "ssh_conn" }
to   = { node = "check_nginx",   port = "server"   }

[[edges]]
from = { node = "check_nginx",   port = "done"     }
to   = { node = "signal_1",      port = "trigger"  }

# Over 链路：over_1 → stop_nginx
[[edges]]
from = { node = "over_1",        port = "signal"   }
to   = { node = "stop_nginx",    port = "trigger"  }

[[edges]]
from = { node = "ssh_conn",      port = "ssh_conn" }
to   = { node = "stop_nginx",    port = "server"   }
```

### 13.1 环境变量引用

```toml
password = "${ENV:SERVER_PASSWORD}"
```

### 13.2 节点 position 字段

`position = { x, y }` 是前端画布坐标，后端原样存储和回传，不参与执行逻辑。所有节点都必须有 position 字段（前端新建时填充）。

### 13.3 关于 `trigger` 端口

业务节点除了显式声明的输入端口（如 `server`）外，框架自动为所有节点提供一个隐式的 `trigger: FlowSignal` 输入端口，用于接收事件源/Flow 节点的信号。节点只有当 `trigger` 被信号激活时才会执行。

孤立节点（没有 trigger 来源）永远不会执行——这就是"从事件源出发可达性分析"的实质。

---

## 14. 开发阶段规划

### 阶段一：最小可运行（MVP）

- [ ] `internal/core` 共享类型定义（不含 Stage 字段）
- [ ] `internal/framework/interfaces.go` Interface 定义
- [ ] `internal/nodes/flow/system_nodes.go` 3 个事件源节点 stub（仅 Define）
- [ ] `internal/registry/registry.go` 注册表 + 注册 3 个事件源节点
- [ ] `internal/store/workflow_store.go` TOML 读写
- [ ] `internal/api/` 三个接口：`POST /workflows`、`GET /workflows/:id`、`GET /node-types`、`PUT /workflows/:id`
- [ ] `cmd/ops-engine/main.go` Gin 启动
- [ ] 前端 `web/`：创建工作流页 + 画布页 + 点击节点显示右侧详情

**目标：** 通过前端创建工作流，画布上能看到 3 个事件源节点，点击可看详情（不涉及执行）

### 阶段二：执行引擎与完整节点

- [ ] `internal/framework/runner.go` 框架编排（RemoteCmd + Flow 分支）
- [ ] `internal/nodes/connection/linux_ssh.go` 第一个业务节点
- [ ] `internal/runtime/executor.go` 基于事件源可达性分析的执行引擎
- [ ] `internal/runtime/handle_store.go` Handle 内存存储
- [ ] `internal/runtime/loop_controller.go` Break 令牌（按 `bound_event` 隔离）
- [ ] `internal/nodes/flow/` break_guard / break_signal
- [ ] `system_update` 多节点独立 delta 调度
- [ ] `system_over` 终止时保证触发 + context.Cancel 传播
- [ ] 日志收集与写入

**目标：** 跑通完整工作流：ready → update 循环 → over 优雅退出

### 阶段三：Agent 模式

- [ ] `cmd/ops-agent` 基础框架
- [ ] `agent/tasks/nginx_install.go`
- [ ] `internal/framework/runner.go` Agent 分支（上传/启动/心跳/回调）
- [ ] `internal/api/handlers.go` `/api/agent/callback` 接口
- [ ] AgentCallbackHub 实现
- [ ] 交叉编译脚本

**目标：** Agent 完整链路跑通

### 阶段四：子工作流 + 完整 API

- [ ] 子工作流激活状态机
- [ ] 参数注入与端口映射
- [ ] 级联终止（由内向外 Over）
- [ ] 完整 REST API
- [ ] WebSocket 推送
- [ ] `nginx_with_docker` + `nginx_with_k8s`

**目标：** 完整后端可用

### 阶段五：前端展示层

- [ ] React Flow 画布（只读展示）
- [ ] WebSocket 订阅，节点状态实时更新
- [ ] ConfigSchema 驱动动态表单
- [ ] 执行日志查看
- [ ] 工作流控制面板

---

*文档版本：v0.2（Go 版，Blueprint 风格事件源节点） | 最后更新：2026-05-23*
