# OpsEngine 开发指南（Go 版）

> 面向 Windows 开发者，从零开始实现运维工作流引擎

---

## 目录

1. [Go 基础概念速览](#1-go-基础概念速览)
2. [Windows 开发环境搭建](#2-windows-开发环境搭建)
3. [创建项目](#3-创建项目)
4. [实现 core 共享类型](#4-实现-core-共享类型)
5. [实现节点开发框架](#5-实现节点开发框架)
6. [实现节点注册表](#6-实现节点注册表)
7. [实现第一个节点：linux_ssh_connection](#7-实现第一个节点linux_ssh_connection)
8. [实现 HandleStore](#8-实现-handlestore)
9. [实现工作流执行引擎（Ready 阶段）](#9-实现工作流执行引擎ready-阶段)
10. [实现文件存储层](#10-实现文件存储层)
11. [实现 REST API](#11-实现-rest-api)
12. [实现 WebSocket 推送](#12-实现-websocket-推送)
13. [实现 UPDATE 循环与 Break 机制](#13-实现-update-循环与-break-机制)
14. [实现 OVER 阶段与终止机制](#14-实现-over-阶段与终止机制)
15. [实现 ops-agent](#15-实现-ops-agent)
16. [实现 AgentNode 框架分支](#16-实现-agentnode-框架分支)
17. [实现子工作流机制](#17-实现子工作流机制)
18. [实现 nginx_with_linux 节点](#18-实现-nginx_with_linux-节点)
19. [运行与测试](#19-运行与测试)

---

## 1. Go 基础概念速览

Go 是一门简单直接的语言，如果你有其他语言经验，快速对照一下关键概念。

### 1.1 基础语法对照

```go
// 变量声明（Go 有类型推断）
name := "hello"           // 自动推断为 string
var age int = 25          // 显式类型
const Pi = 3.14           // 常量

// 函数（支持多返回值，这是 Go 错误处理的基础）
func divide(a, b int) (int, error) {
    if b == 0 {
        return 0, fmt.Errorf("除数不能为零")
    }
    return a / b, nil
}

// 调用时必须处理 error
result, err := divide(10, 2)
if err != nil {
    log.Fatal(err)
}

// struct（类似其他语言的 class，但没有继承）
type User struct {
    Name string
    Age  int
}

// 方法（绑定到 struct 上）
func (u *User) Greet() string {
    return fmt.Sprintf("你好，我是 %s", u.Name)
}

// 创建实例
user := &User{Name: "张三", Age: 25}
fmt.Println(user.Greet())
```

### 1.2 interface（接口）

Go 的接口是**鸭子类型**，不需要显式声明"我实现了这个接口"：

```go
// 定义接口
type Animal interface {
    Name() string
    Speak() string
}

// 实现接口（不需要写 implements）
type Dog struct{}

func (d *Dog) Name() string  { return "狗" }
func (d *Dog) Speak() string { return "汪汪" }

// Dog 自动实现了 Animal 接口
var animal Animal = &Dog{}
fmt.Println(animal.Speak())

// 类型断言（判断接口背后的实际类型）
if dog, ok := animal.(*Dog); ok {
    fmt.Println("确实是狗:", dog.Name())
}
```

### 1.3 goroutine 和 channel（Go 的并发核心）

```go
// goroutine：轻量级线程，go 关键字启动
go func() {
    fmt.Println("在后台运行")
}()

// channel：goroutine 之间的通信管道
ch := make(chan string, 1) // 缓冲为 1

go func() {
    ch <- "hello" // 发送
}()

msg := <-ch // 接收（阻塞直到有数据）
fmt.Println(msg)

// select：同时监听多个 channel
select {
case msg := <-ch1:
    fmt.Println("来自 ch1:", msg)
case msg := <-ch2:
    fmt.Println("来自 ch2:", msg)
case <-time.After(5 * time.Second):
    fmt.Println("超时")
}
```

### 1.4 context（超时与取消）

```go
// context 是 Go 标准的取消传播机制
ctx, cancel := context.WithCancel(context.Background())

// 在 goroutine 中监听取消
go func() {
    select {
    case <-ctx.Done():
        fmt.Println("任务被取消")
        return
    case <-time.After(10 * time.Second):
        fmt.Println("任务完成")
    }
}()

// 取消（类似 Rust 的 CancellationToken）
cancel()
```

### 1.5 defer（延迟执行，保证清理）

```go
// defer 在函数返回时执行，无论是否发生错误
// 类似其他语言的 try-finally
func openFile(path string) {
    f, err := os.Open(path)
    if err != nil {
        return
    }
    defer f.Close() // 无论后续发生什么，函数返回时一定执行

    // ... 处理文件
}
```

### 1.6 常用类型对照

| 其他语言 | Go | 说明 |
|---------|-----|------|
| `string` | `string` | 不可变字符串 |
| `int` | `int` / `int64` | 平台相关 / 固定64位 |
| `bool` | `bool` | 一样 |
| `null` | `nil` | 指针/接口/slice/map 的零值 |
| `try-catch` | `if err != nil` | 多返回值错误处理 |
| `interface` | `interface` | 鸭子类型 |
| `class` | `struct` + `方法` | 数据和方法分开 |
| `HashMap` | `map[K]V` | 内置类型 |
| `ArrayList` | `[]T`（slice） | 内置类型 |
| `thread` | `goroutine` | 更轻量，由运行时管理 |

---

## 2. Windows 开发环境搭建

### 2.1 安装 Go

1. 访问 https://go.dev/dl/ 下载 Windows 安装包（`.msi`）
2. 双击安装，默认路径即可
3. 打开 PowerShell 验证：

```powershell
go version
# 应显示：go version go1.22.x windows/amd64
```

### 2.2 配置 Go 环境

```powershell
# 设置 GOPATH（Go 工作目录，默认 C:\Users\你的用户名\go）
# 通常不需要修改，验证一下即可
go env GOPATH

# 设置国内代理（国内下载依赖用，可选但推荐）
go env -w GOPROXY=https://goproxy.cn,direct
```

### 2.3 安装 VSCode

1. 下载安装 VSCode：https://code.visualstudio.com/
2. 安装 Go 扩展：在扩展市场搜索 `Go`（Google 官方出品）
3. 打开 VSCode，按 `Ctrl+Shift+P`，输入 `Go: Install/Update Tools`，全选安装

安装完后 VSCode 支持：
- 自动补全
- 错误提示
- 跳转定义
- 自动格式化（保存时自动运行 `gofmt`）

### 2.4 安装 Git

```powershell
winget install Git.Git
# 或者去 https://git-scm.com/download/win 下载安装
```

### 2.5 安装 make（可选，运行 Makefile）

```powershell
# 通过 Chocolatey 安装
Set-ExecutionPolicy Bypass -Scope Process -Force
iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
choco install make
```

### 2.6 验证环境

打开 PowerShell，执行以下命令确认环境就绪：

```powershell
go version      # go version go1.22.x
git --version   # git version 2.x.x
code --version  # VSCode 版本
```

---

## 3. 创建项目

### 3.1 初始化项目

打开 PowerShell，创建项目目录：

```powershell
# 在你想要的位置创建项目，比如桌面
cd $HOME\Desktop
mkdir ops-engine
cd ops-engine

# 初始化 Go module（类似 npm init）
go mod init ops-engine

# 初始化 git
git init
```

### 3.2 创建目录结构

```powershell
# 创建所有目录
mkdir cmd\ops-engine
mkdir cmd\ops-agent
mkdir internal\core
mkdir internal\framework
mkdir internal\nodes\connection
mkdir internal\nodes\nginx
mkdir internal\nodes\flow
mkdir internal\registry
mkdir internal\runtime
mkdir internal\store
mkdir internal\api
mkdir agent\tasks
mkdir agents\linux_amd64
mkdir agents\linux_arm64
mkdir data\workflows
mkdir data\executions
mkdir data\logs
```

### 3.3 创建 .gitignore

```powershell
@"
# 编译产物
/ops-engine.exe
/ops-agent.exe
/agents/

# 运行时数据
/data/executions/
/data/logs/

# IDE
.idea/
.vscode/settings.json
"@ | Out-File -FilePath .gitignore -Encoding utf8
```

### 3.4 安装依赖

```powershell
# 安装所有依赖
go get github.com/gin-gonic/gin
go get github.com/gorilla/websocket
go get golang.org/x/crypto/ssh
go get github.com/BurntSushi/toml
go get github.com/google/uuid
go get go.uber.org/zap

# 验证 go.mod 已更新
cat go.mod
```

### 3.5 创建 Makefile

在项目根目录创建 `Makefile`：

```makefile
.PHONY: build run build-agent cross-compile clean

# 编译后端服务
build:
	go build -o ops-engine.exe ./cmd/ops-engine

# 运行（开发模式）
run:
	go run ./cmd/ops-engine

# 编译 Agent（本地测试用）
build-agent:
	go build -o ops-agent.exe ./cmd/ops-agent

# 交叉编译 Agent 到 Linux
cross-compile:
	set GOOS=linux&& set GOARCH=amd64&& go build -o agents/linux_amd64/ops-agent ./cmd/ops-agent
	set GOOS=linux&& set GOARCH=arm64&& go build -o agents/linux_arm64/ops-agent ./cmd/ops-agent

# 清理
clean:
	del /f ops-engine.exe ops-agent.exe 2>nul
	rmdir /s /q agents\linux_amd64 agents\linux_arm64 2>nul
	mkdir agents\linux_amd64 agents\linux_arm64

# 运行测试
test:
	go test ./...

# 代码格式化
fmt:
	go fmt ./...
```

### 3.6 用 VSCode 打开项目

```powershell
code .
```

VSCode 打开后，右下角会提示安装 Go 工具，点击安装即可。

---

## 4. 实现 core 共享类型

`core` 包只定义数据结构，不包含任何业务逻辑和 IO 操作。

### 4.1 创建基础类型文件

创建 `internal/core/types.go`：

```go
package core

import (
	"time"

	"github.com/google/uuid"
)

// ── 端口类型 ──────────────────────────────────────────────

type PortType string

const (
	PortTypeLinuxSsh      PortType = "LinuxSshConnection"
	PortTypeDockerContext  PortType = "DockerContext"
	PortTypeK8sContext     PortType = "K8sContext"
	PortTypeNginxInstance  PortType = "NginxInstance"
	PortTypeFlowSignal     PortType = "FlowSignal"
)

// ── 节点状态 ──────────────────────────────────────────────

type NodeState string

const (
	NodeStateIdle        NodeState = "Idle"
	NodeStateConfiguring NodeState = "Configuring"
	NodeStateChecking    NodeState = "Checking"
	NodeStateCheckFailed NodeState = "CheckFailed"
	NodeStateReady       NodeState = "Ready"
	NodeStateExecuting   NodeState = "Executing"
	NodeStateSuccess     NodeState = "Success"
	NodeStateFailed      NodeState = "Failed"
	NodeStateSkipped     NodeState = "Skipped"
)

// ── 节点不再带 Stage 字段 ─────────────────────────────────
// 阶段归属由"被哪个事件源节点（system_ready / system_update / system_over）可达"导出
// 详见技术设计文档 §5.1

// ── 执行模式 ──────────────────────────────────────────────

type ExecutionMode string

const (
	ExecutionModeRemoteCmd ExecutionMode = "remote_cmd"
	ExecutionModeAgent     ExecutionMode = "agent"
	ExecutionModeFlow      ExecutionMode = "flow"
)

// ── 工作流状态 ────────────────────────────────────────────

type WorkflowStatus string

const (
	WorkflowStatusIdle       WorkflowStatus = "Idle"
	WorkflowStatusRunning    WorkflowStatus = "Running"
	WorkflowStatusTerminated WorkflowStatus = "Terminated"
)

type WorkflowPhase string

const (
	WorkflowPhaseReady  WorkflowPhase = "Ready"
	WorkflowPhaseUpdate WorkflowPhase = "Update"
	WorkflowPhaseOver   WorkflowPhase = "Over"
)

// ── 子工作流激活状态 ──────────────────────────────────────

type ActivationStatus string

const (
	ActivationDormant     ActivationStatus = "Dormant"
	ActivationActivating  ActivationStatus = "Activating"
	ActivationActivated   ActivationStatus = "Activated"
	ActivationRunningOver ActivationStatus = "RunningOver"
)

// ── 时间戳辅助 ────────────────────────────────────────────

type TimeRange struct {
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}
```

### 4.2 创建节点类型定义

创建 `internal/core/node.go`：

```go
package core

import "github.com/google/uuid"

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
	ExecutionMode ExecutionMode `json:"execution_mode"`
}

// 端口定义
type PortDef struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	PortType PortType `json:"port_type"`
	Required bool     `json:"required"`
}

// 配置字段 Schema（驱动前端表单渲染）
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

// 节点实例（工作流配置中的具体节点）
// 注意：无 Stage 字段，生命周期阶段由可达性分析导出
type NodeInstance struct {
	InstanceID uuid.UUID      `json:"instance_id" toml:"id"`
	TypeID     string         `json:"type_id"     toml:"type"`
	Config     map[string]any `json:"config"      toml:"config"`
	State      NodeState      `json:"state"       toml:"-"`
	ErrorMsg   string         `json:"error_msg,omitempty" toml:"-"`
	Position   Position       `json:"position"    toml:"position"`
}

type Position struct {
	X float64 `json:"x" toml:"x"`
	Y float64 `json:"y" toml:"y"`
}
```

### 4.3 创建工作流定义

创建 `internal/core/workflow.go`：

```go
package core

import "github.com/google/uuid"

// UPDATE 阶段配置
type UpdateConfig struct {
	DeltaType    string `json:"delta_type"    toml:"delta_type"`    // interval|cron|manual
	DeltaSeconds int64  `json:"delta_seconds" toml:"delta_seconds"`
	CronExpr     string `json:"cron_expr,omitempty" toml:"cron_expr"`
}

// 连线定义
type Edge struct {
	EdgeID       uuid.UUID `json:"edge_id"`
	SourceNodeID uuid.UUID `json:"source_node_id"`
	SourcePortID string    `json:"source_port_id"`
	TargetNodeID uuid.UUID `json:"target_node_id"`
	TargetPortID string    `json:"target_port_id"`
}

// TOML 配置中连线的简化格式
type EdgeConfig struct {
	From PortRef `toml:"from"`
	To   PortRef `toml:"to"`
}

type PortRef struct {
	Node string `toml:"node"`
	Port string `toml:"port"`
}

// 工作流定义（持久化到 TOML）
// 注意：
//   - 无 UpdateConfig 字段（Delta 配置移到 system_update 节点的 Config 中）
//   - 无 SubWorkflows 字段（子工作流通过 sub_workflow_call 节点表达）
type WorkflowDef struct {
	ID          string         `json:"id"          toml:"id"`
	Name        string         `json:"name"        toml:"name"`
	Description string         `json:"description" toml:"description"`
	Nodes       []NodeInstance `json:"nodes"       toml:"nodes"`
	Edges       []EdgeConfig   `json:"edges"       toml:"edges"`
}
```

> `UpdateConfig` 结构体可以删除——Delta 配置作为 `system_update` 节点的 `config` 字段（普通 `map[string]any`）存储。

### 4.4 创建 Handle 定义

创建 `internal/core/handle.go`：

```go
package core

// HandleValue 是所有 Handle 的基础接口
// 实际连接对象只存在于后端内存
type HandleValue interface {
	HandleType() PortType
}

// SSH 连接 Handle
type SshHandle struct {
	// Client 是实际的 SSH 连接，不序列化（不传给前端）
	Client   any    `json:"-"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
}

func (s *SshHandle) HandleType() PortType { return PortTypeLinuxSsh }

// Nginx 实例 Handle
type NginxHandle struct {
	Version    string `json:"version"`
	ConfigPath string `json:"config_path"`
}

func (n *NginxHandle) HandleType() PortType { return PortTypeNginxInstance }

// 流控信号 Handle
type FlowSignalHandle struct{}

func (f *FlowSignalHandle) HandleType() PortType { return PortTypeFlowSignal }

// HandleMap：节点间传递 Handle 的容器
type HandleMap map[string]HandleValue
```

### 4.5 创建 Agent 相关类型

创建 `internal/core/agent.go`：

```go
package core

// OpsEngine 发给 Agent 的任务描述（序列化为 task.json）
type AgentTask struct {
	TaskID      string         `json:"task_id"`
	TaskType    string         `json:"task_type"`
	CallbackURL string         `json:"callback_url"`
	Config      map[string]any `json:"config"`
}

// Agent 执行完成后回调的数据
type AgentCallback struct {
	TaskID   string         `json:"task_id"`
	Status   string         `json:"status"`   // success | failed
	ErrorMsg string         `json:"error_msg,omitempty"`
	Log      string         `json:"log"`
	Output   map[string]any `json:"output"`
}
```

---

## 5. 实现节点开发框架

### 5.1 创建框架 Interface 定义

创建 `internal/framework/interfaces.go`：

```go
package framework

import (
	"context"
	"ops-engine/internal/core"
)

// HandleMap 是 core.HandleMap 的别名，框架内部使用
type HandleMap = core.HandleMap

// ── ValidateResult ────────────────────────────────────────

type ValidateResult struct {
	Passed bool
	Errors []FieldError
}

type FieldError struct {
	Field   string
	Message string
}

func ValidationOK() ValidateResult {
	return ValidateResult{Passed: true}
}

func ValidationErr(field, message string) ValidateResult {
	return ValidateResult{
		Passed: false,
		Errors: []FieldError{{Field: field, Message: message}},
	}
}

// ── CheckItem ─────────────────────────────────────────────

type CheckItem struct {
	Label    string
	Required bool
	Action   func() CheckItemResult
}

type CheckItemResult struct {
	Passed bool
	Detail string
}

func CheckPass() CheckItemResult             { return CheckItemResult{Passed: true} }
func CheckPassWith(d string) CheckItemResult { return CheckItemResult{Passed: true, Detail: d} }
func CheckFail(d string) CheckItemResult     { return CheckItemResult{Passed: false, Detail: d} }

type CheckResult struct {
	Passed bool
	Items  []CheckItemOutcome
}

type CheckItemOutcome struct {
	Label  string
	Passed bool
	Detail string
}

// ── RemoteCmd ─────────────────────────────────────────────

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

// ── UploadFile ────────────────────────────────────────────

type UploadFile struct {
	Type       string // "agent_binary" | "task_config" | "custom"
	RemotePath string
	Content    []byte
}

func AgentBinaryFile() UploadFile {
	return UploadFile{Type: "agent_binary"}
}

func TaskConfigFile(config map[string]any) UploadFile {
	return UploadFile{Type: "task_config"}
}

// ── ExecContext ───────────────────────────────────────────

type ExecContext struct {
	ExecutionID string
	NodeID      string
	EngineAddr  string
	Logger      *Logger
	Ctx         context.Context
}

// ── Logger ────────────────────────────────────────────────

type Logger struct {
	lines []string
}

func NewLogger() *Logger { return &Logger{} }

func (l *Logger) Log(msg string) {
	line := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	l.lines = append(l.lines, line)
}

func (l *Logger) GetLog() string {
	return strings.Join(l.lines, "\n")
}

// ── Node Interfaces ───────────────────────────────────────

// NodeBase：所有节点必须实现
type NodeBase interface {
	Define() core.NodeTypeDef
	Validate(config map[string]any) ValidateResult
}

// CheckableNode：带检查项的节点
type CheckableNode interface {
	NodeBase
	CheckItems(config map[string]any, inputs HandleMap) []CheckItem
}

// RemoteCmdNode：远程指令执行模式
type RemoteCmdNode interface {
	CheckableNode
	ExecuteCmds(config map[string]any, inputs HandleMap) []RemoteCmd
	OnOutput(config map[string]any, outputs []CmdOutput) (HandleMap, error)
}

// AgentNode：Agent 上传执行模式
type AgentNode interface {
	CheckableNode
	Prepare(config map[string]any) []UploadFile
	TaskConfig(config map[string]any) map[string]any
	OnCallback(result core.AgentCallback) (HandleMap, error)
}

// FlowNode：流控节点
type FlowNode interface {
	NodeBase
	ExecuteFlow(ctx context.Context, config map[string]any, execCtx *ExecContext) (HandleMap, error)
}
```

> **注意**：需要在文件头部添加 import：
> ```go
> import (
>     "fmt"
>     "strings"
>     "time"
>     "context"
>     "ops-engine/internal/core"
> )
> ```

### 5.2 实现检查项执行器

创建 `internal/framework/checker.go`：

```go
package framework

// RunChecks 顺序执行所有检查项，收集结果
// 框架调用，开发者不需要关心这个文件
func RunChecks(items []CheckItem) CheckResult {
	var outcomes []CheckItemOutcome
	allPassed := true

	for _, item := range items {
		result := item.Action()

		if !result.Passed && item.Required {
			allPassed = false
		}

		outcomes = append(outcomes, CheckItemOutcome{
			Label:  item.Label,
			Passed: result.Passed,
			Detail: result.Detail,
		})
	}

	return CheckResult{
		Passed: allPassed,
		Items:  outcomes,
	}
}
```

### 5.3 创建框架入口（占位，后续完善）

创建 `internal/framework/runner.go`：

```go
package framework

import (
	"context"
	"fmt"
	"ops-engine/internal/core"
)

// NodeRunner 是框架的统一编排入口
// 根据节点的 ExecutionMode 选择不同的执行路径
type NodeRunner struct {
	EngineAddr  string
	AgentBinDir string // agents 目录路径
}

func NewNodeRunner(engineAddr, agentBinDir string) *NodeRunner {
	return &NodeRunner{
		EngineAddr:  engineAddr,
		AgentBinDir: agentBinDir,
	}
}

// Run 执行单个节点，返回产出的 HandleMap
func (r *NodeRunner) Run(
	ctx context.Context,
	node NodeBase,
	config map[string]any,
	inputs HandleMap,
	execCtx *ExecContext,
) (HandleMap, error) {

	def := node.Define()
	execCtx.Logger.Log(fmt.Sprintf("开始执行节点: %s", def.TypeID))

	// 1. 配置校验
	validateResult := node.Validate(config)
	if !validateResult.Passed {
		return nil, fmt.Errorf("配置校验失败: %v", validateResult.Errors)
	}

	// 2. 检查阶段
	if checkable, ok := node.(CheckableNode); ok {
		items := checkable.CheckItems(config, inputs)
		result := RunChecks(items)
		if !result.Passed {
			return nil, fmt.Errorf("检查未通过")
		}
	}

	// 3. 执行阶段
	switch def.ExecutionMode {
	case core.ExecutionModeRemoteCmd:
		return r.runRemoteCmd(ctx, node, config, inputs, execCtx)
	case core.ExecutionModeAgent:
		return r.runAgent(ctx, node, config, inputs, execCtx)
	case core.ExecutionModeFlow:
		return r.runFlow(ctx, node, config, inputs, execCtx)
	default:
		return nil, fmt.Errorf("未知执行模式: %s", def.ExecutionMode)
	}
}

func (r *NodeRunner) runRemoteCmd(
	ctx context.Context,
	node NodeBase,
	config map[string]any,
	inputs HandleMap,
	execCtx *ExecContext,
) (HandleMap, error) {
	cmdNode, ok := node.(RemoteCmdNode)
	if !ok {
		return nil, fmt.Errorf("节点声明为 RemoteCmd 但未实现 RemoteCmdNode 接口")
	}

	cmds := cmdNode.ExecuteCmds(config, inputs)
	var outputs []CmdOutput

	for _, cmd := range cmds {
		execCtx.Logger.Log(fmt.Sprintf("执行: %s", cmd.Description))
		output, err := execSSHCmd(inputs, cmd)
		if err != nil {
			return nil, fmt.Errorf("命令执行失败 [%s]: %w", cmd.Cmd, err)
		}
		outputs = append(outputs, output)
	}

	return cmdNode.OnOutput(config, outputs)
}

func (r *NodeRunner) runFlow(
	ctx context.Context,
	node NodeBase,
	config map[string]any,
	inputs HandleMap,
	execCtx *ExecContext,
) (HandleMap, error) {
	flowNode, ok := node.(FlowNode)
	if !ok {
		return nil, fmt.Errorf("节点声明为 Flow 但未实现 FlowNode 接口")
	}
	return flowNode.ExecuteFlow(ctx, config, execCtx)
}

// runAgent 在第16章实现
func (r *NodeRunner) runAgent(
	ctx context.Context,
	node NodeBase,
	config map[string]any,
	inputs HandleMap,
	execCtx *ExecContext,
) (HandleMap, error) {
	// TODO: 第16章实现
	return HandleMap{}, nil
}

// execSSHCmd 通过 SSH Handle 执行命令
func execSSHCmd(inputs HandleMap, cmd RemoteCmd) (CmdOutput, error) {
	// 从 inputs 中找 SSH Handle
	var sshHandle *core.SshHandle
	for _, v := range inputs {
		if h, ok := v.(*core.SshHandle); ok {
			sshHandle = h
			break
		}
	}
	if sshHandle == nil {
		return CmdOutput{}, fmt.Errorf("未找到 SSH Handle")
	}

	// 执行命令（第7章实现真正的 SSH 执行）
	// 这里先返回占位结果
	return CmdOutput{
		Cmd:      cmd.Cmd,
		ExitCode: 0,
	}, nil
}
```

---

## 6. 实现节点注册表

创建 `internal/registry/registry.go`：

```go
package registry

import (
	"ops-engine/internal/core"
	"ops-engine/internal/framework"
	"ops-engine/internal/nodes/connection"
	"ops-engine/internal/nodes/flow"
)

// Registry 存储所有已注册的节点类型
type Registry struct {
	nodes map[string]framework.NodeBase
}

func New() *Registry {
	return &Registry{
		nodes: make(map[string]framework.NodeBase),
	}
}

// Register 注册一个节点类型
func (r *Registry) Register(node framework.NodeBase) {
	def := node.Define()
	r.nodes[def.TypeID] = node
}

// Get 按 TypeID 获取节点
func (r *Registry) Get(typeID string) (framework.NodeBase, bool) {
	node, ok := r.nodes[typeID]
	return node, ok
}

// AllDefs 返回所有节点类型定义（API 用）
func (r *Registry) AllDefs() []core.NodeTypeDef {
	var defs []core.NodeTypeDef
	for _, node := range r.nodes {
		defs = append(defs, node.Define())
	}
	return defs
}

// Build 构建注册表，注册所有内置节点
// 新增节点只需要在这里加一行
func Build() *Registry {
	r := New()
	r.Register(&connection.LinuxSshNode{})
	r.Register(&flow.BreakGuardNode{})
	r.Register(&flow.BreakSignalNode{})
	// r.Register(&nginx.NginxWithLinuxNode{})  // 第18章添加
	return r
}
```

---

## 7. 实现第一个节点：linux_ssh_connection

创建 `internal/nodes/connection/linux_ssh.go`：

```go
package connection

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
	"ops-engine/internal/core"
	"ops-engine/internal/framework"
)

type LinuxSshNode struct{}

// ── NodeBase ──────────────────────────────────────────────

func (n *LinuxSshNode) Define() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "linux_ssh_connection",
		DisplayName: "Linux SSH 连接",
		Category:    "连接",
		Icon:        "🖥️",
		Description: "连接到 Linux 服务器，产出 SSH 连接句柄",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "ssh_conn", Label: "SSH 连接", PortType: core.PortTypeLinuxSsh},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "host", Label: "服务器地址",
				Placeholder: "192.168.1.10", Required: true},
			{Type: "number", ID: "port", Label: "端口",
				Min: int64Ptr(1), Max: int64Ptr(65535), Default: 22},
			{Type: "text", ID: "username", Label: "用户名",
				Placeholder: "root", Required: true},
			{Type: "password", ID: "password", Label: "密码"},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

func (n *LinuxSshNode) Validate(config map[string]any) framework.ValidateResult {
	host, _ := config["host"].(string)
	if host == "" {
		return framework.ValidationErr("host", "服务器地址不能为空")
	}

	port, _ := config["port"].(float64) // JSON 数字默认是 float64
	if port <= 0 || port > 65535 {
		return framework.ValidationErr("port", "端口范围必须在 1-65535 之间")
	}

	username, _ := config["username"].(string)
	if username == "" {
		return framework.ValidationErr("username", "用户名不能为空")
	}

	return framework.ValidationOK()
}

// ── CheckableNode ─────────────────────────────────────────

func (n *LinuxSshNode) CheckItems(
	config map[string]any,
	inputs framework.HandleMap,
) []framework.CheckItem {
	host, _ := config["host"].(string)
	port := int(getFloat(config, "port", 22))
	username, _ := config["username"].(string)
	password, _ := config["password"].(string)

	return []framework.CheckItem{
		{
			Label:    fmt.Sprintf("TCP 端口可达 (%s:%d)", host, port),
			Required: true,
			Action: func() framework.CheckItemResult {
				addr := fmt.Sprintf("%s:%d", host, port)
				conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
				if err != nil {
					return framework.CheckFail(fmt.Sprintf("无法连接: %v", err))
				}
				conn.Close()
				return framework.CheckPassWith("端口可达")
			},
		},
		{
			Label:    "SSH 认证成功",
			Required: true,
			Action: func() framework.CheckItemResult {
				err := trySshAuth(host, port, username, password)
				if err != nil {
					return framework.CheckFail(err.Error())
				}
				return framework.CheckPassWith("认证成功")
			},
		},
	}
}

// ── RemoteCmdNode ─────────────────────────────────────────

func (n *LinuxSshNode) ExecuteCmds(
	config map[string]any,
	inputs framework.HandleMap,
) []framework.RemoteCmd {
	return []framework.RemoteCmd{
		{
			Cmd:         "echo 'ops-engine: connection ok'",
			Description: "验证 SSH 连接",
			TimeoutSecs: 10,
		},
	}
}

func (n *LinuxSshNode) OnOutput(
	config map[string]any,
	outputs []framework.CmdOutput,
) (framework.HandleMap, error) {
	host, _ := config["host"].(string)
	port := int(getFloat(config, "port", 22))
	username, _ := config["username"].(string)
	password, _ := config["password"].(string)

	// 建立真正的 SSH 连接
	client, err := dialSSH(host, port, username, password)
	if err != nil {
		return nil, fmt.Errorf("建立 SSH 连接失败: %w", err)
	}

	return framework.HandleMap{
		"ssh_conn": &core.SshHandle{
			Client:   client,
			Host:     host,
			Port:     port,
			Username: username,
		},
	}, nil
}

// ── 辅助函数 ──────────────────────────────────────────────

// trySshAuth 尝试 SSH 认证（用于检查阶段）
func trySshAuth(host string, port int, username, password string) error {
	client, err := dialSSH(host, port, username, password)
	if err != nil {
		return err
	}
	client.Close()
	return nil
}

// dialSSH 建立 SSH 连接
func dialSSH(host string, port int, username, password string) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		// 开发阶段忽略 host key 检查
		// 生产环境应该用 ssh.FixedHostKey
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败: %w", err)
	}
	return client, nil
}

// ExecSSHCommand 通过已有连接执行命令
func ExecSSHCommand(client *ssh.Client, cmd string) (string, string, int, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", "", -1, fmt.Errorf("创建 session 失败: %w", err)
	}
	defer session.Close()

	var stdout, stderr strings.Builder
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(cmd)
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			return "", "", -1, err
		}
	}

	return stdout.String(), stderr.String(), exitCode, nil
}

// 辅助函数
func getFloat(m map[string]any, key string, def float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return def
}

func int64Ptr(v int64) *int64 { return &v }
```

> 需要在文件头 import `strings`。

---

## 8. 实现 HandleStore

创建 `internal/runtime/handle_store.go`：

```go
package runtime

import (
	"sync"

	"github.com/google/uuid"
	"ops-engine/internal/core"
)

// HandleStore 管理所有运行时 Handle 的内存存储
// 工作流终止时统一释放
type HandleStore struct {
	mu    sync.RWMutex
	store map[uuid.UUID]core.HandleValue
}

func NewHandleStore() *HandleStore {
	return &HandleStore{
		store: make(map[uuid.UUID]core.HandleValue),
	}
}

// Insert 存入 Handle，返回分配的 ID
func (s *HandleStore) Insert(value core.HandleValue) uuid.UUID {
	id := uuid.New()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[id] = value
	return id
}

// Get 获取 Handle
func (s *HandleStore) Get(id uuid.UUID) (core.HandleValue, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.store[id]
	return v, ok
}

// Clear 释放所有 Handle（工作流终止时调用）
func (s *HandleStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 关闭所有 SSH 连接
	for _, v := range s.store {
		if sshHandle, ok := v.(*core.SshHandle); ok {
			if client, ok := sshHandle.Client.(interface{ Close() error }); ok {
				client.Close()
			}
		}
	}

	s.store = make(map[uuid.UUID]core.HandleValue)
}

func (s *HandleStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.store)
}
```

---

## 9. 实现工作流执行引擎

> ⚠️ **本章及后续 §13、§14 中的执行引擎实现需要按 Blueprint 风格事件源模型重新设计。**
>
> 关键变化（参见技术设计文档 §5.1 / §5.2 / §13）：
> - 节点不再有 `Stage` 字段，按"被哪个事件源可达"决定何时执行
> - `runPhase(stage)` / `filterNodes(stage)` 这类按阶段过滤的方法**整体废弃**
> - 改为：执行器扫描所有 `system_ready` / `system_update` / `system_over` 节点，分别从它们出发做拓扑遍历
> - 每个 `system_update` 节点独立调度（独立 ticker + 独立 LoopController）
> - `defer e.fireSystemOverNodes()` 保证终止时触发所有 over 节点的下游
>
> 下面给出的代码示例仍基于旧的"runPhase + Stage 过滤"思路，请视作参考骨架，按新模型重写。

创建 `internal/runtime/executor.go`：

```go
package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"ops-engine/internal/core"
	"ops-engine/internal/framework"
	"ops-engine/internal/registry"
	"ops-engine/internal/store"
)

// WorkflowExecutor 管理一个工作流实例的完整生命周期
type WorkflowExecutor struct {
	Def         core.WorkflowDef
	Status      core.WorkflowStatus
	Phase       core.WorkflowPhase
	ExecutionID string

	ctx    context.Context
	cancel context.CancelFunc

	handleStore    *HandleStore
	logCollector   *LogCollector
	runner         *framework.NodeRunner
	registry       *registry.Registry
	executionStore *store.ExecutionStore
	logger         *zap.Logger

	terminateReason string
	startedAt       time.Time
}

func NewExecutor(
	def core.WorkflowDef,
	reg *registry.Registry,
	executionStore *store.ExecutionStore,
	engineAddr string,
) *WorkflowExecutor {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkflowExecutor{
		Def:            def,
		Status:         core.WorkflowStatusIdle,
		Phase:          core.WorkflowPhaseReady,
		ExecutionID:    uuid.New().String(),
		ctx:            ctx,
		cancel:         cancel,
		handleStore:    NewHandleStore(),
		logCollector:   NewLogCollector(),
		runner:         framework.NewNodeRunner(engineAddr, "agents"),
		registry:       reg,
		executionStore: executionStore,
		logger:         zap.L(),
	}
}

// Run 启动工作流
func (e *WorkflowExecutor) Run() error {
	e.Status = core.WorkflowStatusRunning
	e.startedAt = time.Now()
	e.logger.Info("工作流开始执行", zap.String("name", e.Def.Name))

	// defer 保证无论成功失败都执行 Over 和清理
	defer e.cleanup()

	// 执行 Ready 阶段
	e.Phase = core.WorkflowPhaseReady
	if err := e.runPhase(core.NodeStageReady); err != nil {
		e.logger.Error("Ready 阶段失败", zap.Error(err))
		e.Terminate(err.Error())
		return err
	}

	e.logger.Info("Ready 阶段完成，准备进入 Update 循环")
	// Update 循环和 Over 阶段在后续章节实现

	e.Status = core.WorkflowStatusTerminated
	return nil
}

// Terminate 强制终止工作流
func (e *WorkflowExecutor) Terminate(reason string) {
	e.terminateReason = reason
	e.cancel() // 广播终止信号到所有 goroutine
	e.logger.Warn("工作流终止", zap.String("reason", reason))
}

// runPhase 执行指定阶段的所有节点
func (e *WorkflowExecutor) runPhase(stage core.NodeStage) error {
	nodes := e.filterNodes(stage)
	if len(nodes) == 0 {
		return nil
	}

	// 按拓扑排序执行
	ordered := e.topologicalSort(nodes)

	// 收集已产出的 Handle，供下游节点使用
	available := make(core.HandleMap)

	for _, node := range ordered {
		// 检查是否已被终止
		select {
		case <-e.ctx.Done():
			return fmt.Errorf("工作流已终止")
		default:
		}

		e.logger.Info("执行节点", zap.String("type", node.TypeID))

		// 从 Registry 找节点实现
		impl, ok := e.registry.Get(node.TypeID)
		if !ok {
			return fmt.Errorf("节点类型未注册: %s", node.TypeID)
		}

		// 构建该节点的输入 Handle
		inputs := e.resolveInputs(node.InstanceID, available)

		// 构建执行上下文
		logger := framework.NewLogger()
		execCtx := &framework.ExecContext{
			ExecutionID: e.ExecutionID,
			NodeID:      node.InstanceID.String(),
			EngineAddr:  e.runner.EngineAddr,
			Logger:      logger,
			Ctx:         e.ctx,
		}

		// 执行节点
		outputs, err := e.runner.Run(e.ctx, impl, node.Config, inputs, execCtx)

		// 日志暂存
		e.logCollector.Append(node.InstanceID, logger.GetLog())

		if err != nil {
			return fmt.Errorf("节点 %s 执行失败: %w", node.TypeID, err)
		}

		// 将产出的 Handle 加入可用集合
		for portID, handle := range outputs {
			key := fmt.Sprintf("%s:%s", node.InstanceID, portID)
			available[key] = handle
		}

		e.logger.Info("节点执行成功", zap.String("type", node.TypeID))
	}

	return nil
}

// filterNodes 过滤出指定阶段的节点
func (e *WorkflowExecutor) filterNodes(stage core.NodeStage) []core.NodeInstance {
	var nodes []core.NodeInstance
	for _, n := range e.Def.Nodes {
		if n.Stage == stage {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

// resolveInputs 根据连线找到节点的输入 Handle
func (e *WorkflowExecutor) resolveInputs(nodeID uuid.UUID, available core.HandleMap) core.HandleMap {
	inputs := make(core.HandleMap)
	for _, edge := range e.Def.Edges {
		// 找目标节点的名字对应的 UUID（简化版，实际需要按 ID 查）
		// TODO: 完善边的 ID 解析
		_ = edge
	}
	return inputs
}

// topologicalSort 按依赖关系排列节点（简化版）
func (e *WorkflowExecutor) topologicalSort(nodes []core.NodeInstance) []core.NodeInstance {
	// 简单实现：按节点在配置中的顺序执行
	// TODO: 实现基于 edges 的真正拓扑排序
	return nodes
}

// cleanup 工作流结束时的清理工作
func (e *WorkflowExecutor) cleanup() {
	e.logger.Info("开始清理工作流资源")

	// 执行 Over 阶段（第14章实现）
	// e.runPhase(core.NodeStageOver)

	// 释放所有 Handle
	e.handleStore.Clear()

	// 保存执行记录
	e.saveExecutionRecord()

	e.Status = core.WorkflowStatusTerminated
	e.logger.Info("工作流清理完成")
}

// saveExecutionRecord 保存执行记录到文件
func (e *WorkflowExecutor) saveExecutionRecord() {
	// TODO: 调用 executionStore 保存记录和日志
}
```

---

## 10. 实现文件存储层

### 10.1 实现工作流存储

创建 `internal/store/workflow_store.go`：

```go
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"ops-engine/internal/core"
)

type WorkflowStore struct {
	baseDir string
}

func NewWorkflowStore(baseDir string) *WorkflowStore {
	return &WorkflowStore{baseDir: baseDir}
}

// List 列出所有工作流
func (s *WorkflowStore) List() ([]core.WorkflowDef, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, err
	}

	var workflows []core.WorkflowDef
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".toml") {
			wf, err := s.Get(strings.TrimSuffix(entry.Name(), ".toml"))
			if err == nil {
				workflows = append(workflows, wf)
			}
		}
	}
	return workflows, nil
}

// Get 按 ID 加载工作流
func (s *WorkflowStore) Get(id string) (core.WorkflowDef, error) {
	path := filepath.Join(s.baseDir, id+".toml")
	content, err := os.ReadFile(path)
	if err != nil {
		return core.WorkflowDef{}, fmt.Errorf("工作流未找到: %s", id)
	}

	// 处理环境变量引用
	content = []byte(resolveEnvVars(string(content)))

	var wf core.WorkflowDef
	if _, err := toml.Decode(string(content), &wf); err != nil {
		return core.WorkflowDef{}, fmt.Errorf("TOML 解析失败: %w", err)
	}
	return wf, nil
}

// Save 保存工作流（JSON 格式，方便 API 创建）
func (s *WorkflowStore) Save(wf core.WorkflowDef) error {
	// 注意：API 创建的工作流保存为 JSON，
	// 手动编写的工作流用 TOML 格式
	// 这里简化为直接用 toml 编码
	path := filepath.Join(s.baseDir, wf.ID+".toml")

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(wf)
}

// Delete 删除工作流
func (s *WorkflowStore) Delete(id string) error {
	path := filepath.Join(s.baseDir, id+".toml")
	return os.Remove(path)
}

// resolveEnvVars 替换 ${ENV:VAR_NAME} 格式的环境变量引用
func resolveEnvVars(content string) string {
	for {
		start := strings.Index(content, "${ENV:")
		if start == -1 {
			break
		}
		end := strings.Index(content[start:], "}")
		if end == -1 {
			break
		}
		placeholder := content[start : start+end+1]
		varName := content[start+6 : start+end]
		value := os.Getenv(varName)
		content = strings.Replace(content, placeholder, value, 1)
	}
	return content
}
```

### 10.2 实现执行历史存储

创建 `internal/store/execution_store.go`：

```go
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ExecutionRecord struct {
	ID              string               `json:"id"`
	WorkflowID      string               `json:"workflow_id"`
	WorkflowName    string               `json:"workflow_name"`
	Status          string               `json:"status"`
	TerminateReason string               `json:"terminate_reason,omitempty"`
	StartedAt       time.Time            `json:"started_at"`
	FinishedAt      *time.Time           `json:"finished_at,omitempty"`
	Nodes           []NodeExecutionRecord `json:"nodes"`
}

type NodeExecutionRecord struct {
	ID          string     `json:"id"`
	NodeType    string     `json:"node_type"`
	TriggeredBy string     `json:"triggered_by"` // 触发本次执行的事件源节点 ID
	Status      string     `json:"status"`
	LoopCount   *int       `json:"loop_count,omitempty"`
	ErrorMsg    string     `json:"error_msg,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

type ExecutionStore struct {
	executionsDir string
	logsDir       string
}

func NewExecutionStore(executionsDir, logsDir string) *ExecutionStore {
	return &ExecutionStore{
		executionsDir: executionsDir,
		logsDir:       logsDir,
	}
}

// SaveRecord 保存执行记录
func (s *ExecutionStore) SaveRecord(record ExecutionRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.executionsDir, record.ID+".json")
	return os.WriteFile(path, data, 0644)
}

// SaveLog 保存节点日志
func (s *ExecutionStore) SaveLog(executionID, nodeID, log string) error {
	dir := filepath.Join(s.logsDir, executionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, nodeID+".log")
	return os.WriteFile(path, []byte(log), 0644)
}

// GetLog 读取节点日志
func (s *ExecutionStore) GetLog(executionID, nodeID string) (string, error) {
	path := filepath.Join(s.logsDir, executionID, nodeID+".log")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// List 列出所有执行记录（时间倒序）
func (s *ExecutionStore) List() ([]ExecutionRecord, error) {
	entries, err := os.ReadDir(s.executionsDir)
	if err != nil {
		return nil, err
	}

	var records []ExecutionRecord
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			data, err := os.ReadFile(filepath.Join(s.executionsDir, entry.Name()))
			if err != nil {
				continue
			}
			var record ExecutionRecord
			if err := json.Unmarshal(data, &record); err == nil {
				records = append(records, record)
			}
		}
	}

	// 按时间倒序
	sort.Slice(records, func(i, j int) bool {
		return records[i].StartedAt.After(records[j].StartedAt)
	})
	return records, nil
}
```

---

## 11. 实现 REST API

### 11.1 创建路由和 AppState

创建 `internal/api/rest.go`：

```go
package api

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"ops-engine/internal/registry"
	"ops-engine/internal/runtime"
	"ops-engine/internal/store"
)

// AppState 所有路由共享的应用状态
type AppState struct {
	Registry       *registry.Registry
	WorkflowStore  *store.WorkflowStore
	ExecutionStore *store.ExecutionStore
	// 运行中的工作流，key: workflow_id
	Running    map[string]*runtime.WorkflowExecutor
	RunningMu  sync.Mutex
	EngineAddr string
}

// NewRouter 构建 Gin 路由
func NewRouter(state *AppState) *gin.Engine {
	r := gin.Default()

	// CORS（开发阶段允许所有来源）
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	api := r.Group("/api")

	// 节点类型
	api.GET("/node-types", state.listNodeTypes)

	// 工作流定义
	api.GET("/workflows", state.listWorkflows)
	api.GET("/workflows/:id", state.getWorkflow)
	api.POST("/workflows", state.createWorkflow)
	api.PUT("/workflows/:id", state.updateWorkflow)
	api.DELETE("/workflows/:id", state.deleteWorkflow)

	// 工作流控制
	api.POST("/workflows/:id/start", state.startWorkflow)
	api.POST("/workflows/:id/terminate", state.terminateWorkflow)

	// 执行历史
	api.GET("/executions", state.listExecutions)
	api.GET("/executions/:id", state.getExecution)
	api.GET("/executions/:exec_id/nodes/:node_id/log", state.getNodeLog)

	// Agent 回调
	api.POST("/agent/callback", state.agentCallback)

	// WebSocket
	r.GET("/ws/executions/:id", state.wsHandler)

	return r
}
```

### 11.2 实现各接口处理函数

创建 `internal/api/handlers.go`：

```go
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"ops-engine/internal/core"
	"ops-engine/internal/runtime"
)

func (s *AppState) listNodeTypes(c *gin.Context) {
	c.JSON(http.StatusOK, s.Registry.AllDefs())
}

func (s *AppState) listWorkflows(c *gin.Context) {
	workflows, err := s.WorkflowStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, workflows)
}

func (s *AppState) getWorkflow(c *gin.Context) {
	wf, err := s.WorkflowStore.Get(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, wf)
}

func (s *AppState) createWorkflow(c *gin.Context) {
	var wf core.WorkflowDef
	if err := c.ShouldBindJSON(&wf); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.WorkflowStore.Save(wf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": wf.ID})
}

func (s *AppState) updateWorkflow(c *gin.Context) {
	var wf core.WorkflowDef
	if err := c.ShouldBindJSON(&wf); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	wf.ID = c.Param("id")
	if err := s.WorkflowStore.Save(wf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *AppState) deleteWorkflow(c *gin.Context) {
	if err := s.WorkflowStore.Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *AppState) startWorkflow(c *gin.Context) {
	id := c.Param("id")

	wf, err := s.WorkflowStore.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	s.RunningMu.Lock()
	if _, exists := s.Running[id]; exists {
		s.RunningMu.Unlock()
		c.JSON(http.StatusConflict, gin.H{"error": "工作流已在运行中"})
		return
	}

	executor := runtime.NewExecutor(wf, s.Registry, s.ExecutionStore, s.EngineAddr)
	s.Running[id] = executor
	s.RunningMu.Unlock()

	// 后台异步执行
	go func() {
		executor.Run()
		s.RunningMu.Lock()
		delete(s.Running, id)
		s.RunningMu.Unlock()
	}()

	c.JSON(http.StatusAccepted, gin.H{"message": "工作流已启动", "execution_id": executor.ExecutionID})
}

func (s *AppState) terminateWorkflow(c *gin.Context) {
	id := c.Param("id")
	s.RunningMu.Lock()
	executor, exists := s.Running[id]
	s.RunningMu.Unlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "工作流未在运行"})
		return
	}
	executor.Terminate("用户手动终止")
	c.JSON(http.StatusOK, gin.H{"message": "已发送终止信号"})
}

func (s *AppState) listExecutions(c *gin.Context) {
	records, err := s.ExecutionStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, records)
}

func (s *AppState) getExecution(c *gin.Context) {
	// TODO: 实现单条查询
	c.JSON(http.StatusOK, gin.H{"id": c.Param("id")})
}

func (s *AppState) getNodeLog(c *gin.Context) {
	log, err := s.ExecutionStore.GetLog(c.Param("exec_id"), c.Param("node_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "日志不存在"})
		return
	}
	c.String(http.StatusOK, log)
}

func (s *AppState) agentCallback(c *gin.Context) {
	var callback core.AgentCallback
	if err := c.ShouldBindJSON(&callback); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// TODO: 通知等待中的节点
	c.JSON(http.StatusOK, gin.H{"received": true})
}
```

---

## 12. 实现 WebSocket 推送

创建 `internal/api/ws.go`：

```go
package api

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// WsEvent WebSocket 推送的事件结构
type WsEvent struct {
	Event   string `json:"event"`
	Payload any    `json:"payload"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // 开发阶段允许所有来源
}

// EventHub 管理所有 WebSocket 连接
type EventHub struct {
	mu      sync.Mutex
	clients map[string][]*websocket.Conn // key: execution_id
}

func NewEventHub() *EventHub {
	return &EventHub{
		clients: make(map[string][]*websocket.Conn),
	}
}

// Broadcast 向指定执行的所有订阅者推送事件
func (h *EventHub) Broadcast(executionID string, event WsEvent) {
	data, _ := json.Marshal(event)
	h.mu.Lock()
	defer h.mu.Unlock()

	conns := h.clients[executionID]
	var alive []*websocket.Conn
	for _, conn := range conns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err == nil {
			alive = append(alive, conn)
		}
		// 写失败说明连接已断开，不加入 alive
	}
	h.clients[executionID] = alive
}

// wsHandler 处理 WebSocket 升级
func (s *AppState) wsHandler(c *gin.Context) {
	executionID := c.Param("id")

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	// 注册连接
	s.EventHub.mu.Lock()
	s.EventHub.clients[executionID] = append(s.EventHub.clients[executionID], conn)
	s.EventHub.mu.Unlock()

	// 保持连接，等待客户端断开
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}
```

> 需要在 `AppState` 中添加 `EventHub *EventHub` 字段。

---

## 13. 实现 system_update 循环与 Break 机制

> ⚠️ 按新模型，每个 `system_update` 节点独立维护 ticker 与 LoopController。下面的旧实现是按"工作流级 Update 循环"组织的，请参考思路但按"事件源节点级"重写。

创建 `internal/runtime/loop_controller.go`：

```go
package runtime

import (
	"context"

	"go.uber.org/zap"
)

// LoopController 管理 UPDATE 循环的令牌机制
type LoopController struct {
	// 缓冲为 1，防止重复发令牌
	tokenCh   chan struct{}
	LoopCount int
}

func NewLoopController() *LoopController {
	return &LoopController{
		tokenCh: make(chan struct{}, 1),
	}
}

// EmitToken Break Signal 调用，发出令牌
func (lc *LoopController) EmitToken() {
	select {
	case lc.tokenCh <- struct{}{}:
		zap.L().Debug("Break 令牌已发出", zap.Int("loop_count", lc.LoopCount))
	default:
		// 已有令牌，不重复发送
	}
}

// WaitForToken Break Guard 调用，等待令牌
// 返回 true 表示可以执行，false 表示工作流已终止
func (lc *LoopController) WaitForToken(ctx context.Context) bool {
	// 首轮直接放行
	if lc.LoopCount == 0 {
		return true
	}

	select {
	case <-lc.tokenCh:
		zap.L().Debug("Break Guard 收到令牌，放行")
		return true
	case <-ctx.Done():
		return false
	}
}

// NextLoop 进入下一轮
func (lc *LoopController) NextLoop() {
	lc.LoopCount++
}
```

在 `executor.go` 中添加 UPDATE 循环（在 `Run()` 方法中 Ready 阶段之后）：

```go
// runUpdateLoop UPDATE 阶段循环
func (e *WorkflowExecutor) runUpdateLoop() {
	e.Phase = core.WorkflowPhaseUpdate
	loopCtrl := NewLoopController()

	for {
		// 检查终止信号
		select {
		case <-e.ctx.Done():
			return
		default:
		}

		loopCtrl.NextLoop()
		e.logger.Info("开始 Update 循环", zap.Int("loop", loopCtrl.LoopCount))

		// 等待 Break Guard 令牌
		if !loopCtrl.WaitForToken(e.ctx) {
			return
		}

		// 执行 Update 阶段节点
		if err := e.runPhaseWithLoopCtrl(core.NodeStageUpdate, loopCtrl); err != nil {
			e.logger.Error("Update 阶段失败", zap.Error(err))
			e.Terminate(err.Error())
			return
		}

		// 等待下次 Delta
		if !e.waitDelta() {
			return
		}
	}
}

// waitDelta 等待 Delta 时间间隔
func (e *WorkflowExecutor) waitDelta() bool {
	cfg := e.Def.UpdateConfig
	switch cfg.DeltaType {
	case "manual":
		return false // manual 模式只执行一次
	case "interval":
		secs := cfg.DeltaSeconds
		if secs <= 0 {
			secs = 60
		}
		select {
		case <-time.After(time.Duration(secs) * time.Second):
			return true
		case <-e.ctx.Done():
			return false
		}
	default:
		return false
	}
}
```

---

## 14. 实现终止机制与 system_over 触发

> ⚠️ 按新模型，"Over 阶段"实际是引擎在 `defer` 中触发所有 `system_over` 节点的下游子图。

在 `executor.go` 的 `cleanup()` 方法中添加 Over 阶段执行：

```go
func (e *WorkflowExecutor) cleanup() {
	e.logger.Info("开始执行 Over 阶段")
	e.Phase = core.WorkflowPhaseOver

	// 执行 Over 阶段（带超时）
	overCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := e.runPhaseWithCtx(overCtx, core.NodeStageOver); err != nil {
		e.logger.Error("Over 阶段执行失败", zap.Error(err))
	}

	// 释放所有 Handle
	e.handleStore.Clear()
	e.logger.Info("所有 Handle 已释放")

	// 保存执行记录
	e.saveExecutionRecord()
}
```

---

## 15. 实现 ops-agent

### 15.1 Agent 任务接口

创建 `agent/interfaces.go`：

```go
package agent

import "context"

// AgentTask 所有 Agent 任务必须实现的接口
type AgentTask interface {
	Execute(ctx context.Context, config map[string]any, logger *TaskLogger) AgentTaskResult
}

type AgentTaskResult struct {
	Success  bool
	ErrorMsg string
	Output   map[string]any
}
```

### 15.2 任务日志

创建 `agent/logger.go`：

```go
package agent

import (
	"fmt"
	"strings"
	"time"
)

type TaskLogger struct {
	lines []string
}

func NewTaskLogger() *TaskLogger { return &TaskLogger{} }

func (l *TaskLogger) Log(msg string) {
	line := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	fmt.Println(line)
	l.lines = append(l.lines, line)
}

func (l *TaskLogger) GetLog() string {
	return strings.Join(l.lines, "\n")
}
```

### 15.3 Nginx 安装任务

创建 `agent/tasks/nginx_install.go`：

```go
package tasks

import (
	"context"
	"fmt"
	"os/exec"

	"ops-engine/agent"
)

type NginxInstallTask struct{}

func (t *NginxInstallTask) Execute(
	ctx context.Context,
	config map[string]any,
	logger *agent.TaskLogger,
) agent.AgentTaskResult {
	version, _ := config["version"].(string)
	if version == "" {
		version = "latest"
	}
	start, _ := config["start"].(bool)

	logger.Log(fmt.Sprintf("开始安装 Nginx，版本: %s", version))

	// 检测包管理器
	pkgMgr := detectPkgManager(logger)
	if pkgMgr == "" {
		return agent.AgentTaskResult{
			Success:  false,
			ErrorMsg: "未找到支持的包管理器 (apt/yum)",
		}
	}

	// 安装
	var installErr error
	if pkgMgr == "apt" {
		installErr = runCmd(logger, "apt-get", "install", "-y", "nginx")
	} else {
		installErr = runCmd(logger, "yum", "install", "-y", "nginx")
	}

	if installErr != nil {
		return agent.AgentTaskResult{Success: false, ErrorMsg: "Nginx 安装失败"}
	}

	// 获取版本
	nginxVersion := getNginxVersion(logger)

	// 启动
	if start {
		logger.Log("启动 Nginx...")
		runCmd(logger, "systemctl", "start", "nginx")
		runCmd(logger, "systemctl", "enable", "nginx")
	}

	logger.Log("Nginx 安装完成")

	return agent.AgentTaskResult{
		Success: true,
		Output: map[string]any{
			"nginx_version": nginxVersion,
			"config_path":   "/etc/nginx/nginx.conf",
		},
	}
}

func detectPkgManager(logger *agent.TaskLogger) string {
	if _, err := exec.LookPath("apt-get"); err == nil {
		logger.Log("检测到包管理器: apt")
		return "apt"
	}
	if _, err := exec.LookPath("yum"); err == nil {
		logger.Log("检测到包管理器: yum")
		return "yum"
	}
	logger.Log("错误：未找到支持的包管理器")
	return ""
}

func runCmd(logger *agent.TaskLogger, name string, args ...string) error {
	logger.Log(fmt.Sprintf("执行: %s %v", name, args))
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	logger.Log(string(out))
	if err != nil {
		logger.Log(fmt.Sprintf("失败: %v", err))
	}
	return err
}

func getNginxVersion(logger *agent.TaskLogger) string {
	out, err := exec.Command("nginx", "-v").CombinedOutput()
	if err != nil {
		return "unknown"
	}
	v := string(out)
	logger.Log("Nginx 版本: " + v)
	return v
}
```

### 15.4 回调逻辑

创建 `agent/callback.go`：

```go
package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"ops-engine/internal/core"
)

func SendCallback(
	callbackURL string,
	taskID      string,
	result      AgentTaskResult,
	log         string,
) error {
	status := "success"
	if !result.Success {
		status = "failed"
	}

	payload := core.AgentCallback{
		TaskID:   taskID,
		Status:   status,
		ErrorMsg: result.ErrorMsg,
		Log:      log,
		Output:   result.Output,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(callbackURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("回调请求失败: %w", err)
	}
	defer resp.Body.Close()

	return nil
}
```

### 15.5 Agent main 入口

创建 `cmd/ops-agent/main.go`：

```go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"ops-engine/agent"
	"ops-engine/agent/tasks"
	"ops-engine/internal/core"
)

func main() {
	taskFile   := flag.String("task", "", "任务配置文件路径")
	callbackURL := flag.String("callback", "", "OpsEngine 回调地址")
	taskID     := flag.String("task-id", "", "任务 ID")
	flag.Parse()

	if *taskFile == "" || *callbackURL == "" || *taskID == "" {
		fmt.Println("缺少必要参数")
		os.Exit(1)
	}

	// 读取任务配置
	data, err := os.ReadFile(*taskFile)
	if err != nil {
		fmt.Printf("读取任务文件失败: %v\n", err)
		os.Exit(1)
	}

	var taskDef core.AgentTask
	if err := json.Unmarshal(data, &taskDef); err != nil {
		fmt.Printf("解析任务配置失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("ops-agent 启动，task_id=%s, task_type=%s\n", *taskID, taskDef.TaskType)

	// 写 pid 文件
	pidFile := fmt.Sprintf("/tmp/ops-agent-%s.pid", *taskID)
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
	defer os.Remove(pidFile)

	// 执行任务
	logger := agent.NewTaskLogger()
	ctx := context.Background()

	var task agent.AgentTask
	switch taskDef.TaskType {
	case "nginx_install":
		task = &tasks.NginxInstallTask{}
	default:
		fmt.Printf("未知任务类型: %s\n", taskDef.TaskType)
		os.Exit(1)
	}

	result := task.Execute(ctx, taskDef.Config, logger)

	// 发送回调
	if err := agent.SendCallback(*callbackURL, *taskID, result, logger.GetLog()); err != nil {
		fmt.Printf("回调失败: %v\n", err)
	}

	fmt.Println("ops-agent 退出")
}
```

---

## 16. 实现 AgentNode 框架分支

在 `internal/framework/runner.go` 中实现 `runAgent` 方法：

```go
func (r *NodeRunner) runAgent(
	ctx context.Context,
	node NodeBase,
	config map[string]any,
	inputs HandleMap,
	execCtx *ExecContext,
) (HandleMap, error) {
	agentNode, ok := node.(AgentNode)
	if !ok {
		return nil, fmt.Errorf("节点声明为 Agent 但未实现 AgentNode 接口")
	}

	// 获取 SSH Handle
	sshHandle := findSshHandle(inputs)
	if sshHandle == nil {
		return nil, fmt.Errorf("未找到 SSH Handle")
	}

	sshClient := sshHandle.Client.(*ssh.Client)

	taskID := uuid.New().String()
	taskConfig := agentNode.TaskConfig(config)
	taskConfig["task_id"] = taskID

	callbackURL := fmt.Sprintf("http://%s/api/agent/callback", r.EngineAddr)

	// 1. 检测目标架构
	arch, err := execSSHString(sshClient, "uname -m")
	if err != nil {
		return nil, fmt.Errorf("架构检测失败: %w", err)
	}

	agentBinPath := filepath.Join(r.AgentBinDir, "linux_amd64", "ops-agent")
	if strings.Contains(arch, "arm") || strings.Contains(arch, "aarch64") {
		agentBinPath = filepath.Join(r.AgentBinDir, "linux_arm64", "ops-agent")
	}

	// 2. 上传 Agent binary
	agentBin, err := os.ReadFile(agentBinPath)
	if err != nil {
		return nil, fmt.Errorf("读取 Agent binary 失败: %w", err)
	}

	remoteBin := fmt.Sprintf("/tmp/ops-agent-%s", taskID)
	execCtx.Logger.Log("上传 Agent binary...")
	if err := scpUpload(sshClient, remoteBin, agentBin, 0755); err != nil {
		return nil, fmt.Errorf("上传 Agent 失败: %w", err)
	}

	// 3. 上传 task.json
	taskJSON, _ := json.Marshal(core.AgentTask{
		TaskID:      taskID,
		TaskType:    taskConfig["task_type"].(string),
		CallbackURL: callbackURL,
		Config:      taskConfig,
	})

	remoteTask := fmt.Sprintf("/tmp/task-%s.json", taskID)
	if err := scpUpload(sshClient, remoteTask, taskJSON, 0644); err != nil {
		return nil, fmt.Errorf("上传任务配置失败: %w", err)
	}

	// 4. 启动 Agent
	launchCmd := fmt.Sprintf(
		"nohup %s --task %s --callback %s --task-id %s > /tmp/ops-agent-%s.log 2>&1 &",
		remoteBin, remoteTask, callbackURL, taskID, taskID,
	)
	execCtx.Logger.Log("启动 Agent...")
	if _, err := execSSHString(sshClient, launchCmd); err != nil {
		return nil, fmt.Errorf("启动 Agent 失败: %w", err)
	}

	// 5. 等待回调（带心跳检测）
	execCtx.Logger.Log("等待 Agent 执行完成...")
	callback, err := r.waitAgentCallback(ctx, sshClient, taskID, sshHandle)
	if err != nil {
		return nil, err
	}

	// 6. defer cleanup（Go 的 defer 保证执行）
	defer func() {
		execSSHString(sshClient, fmt.Sprintf("rm -f /tmp/ops-agent-%s /tmp/task-%s.json /tmp/ops-agent-%s.pid /tmp/ops-agent-%s.log", taskID, taskID, taskID, taskID))
	}()

	return agentNode.OnCallback(*callback)
}

// waitAgentCallback 等待 Agent 回调，同时做心跳检测
func (r *NodeRunner) waitAgentCallback(
	ctx context.Context,
	sshClient *ssh.Client,
	taskID string,
	sshHandle *core.SshHandle,
) (*core.AgentCallback, error) {
	callbackCh := r.CallbackHub.Wait(taskID, ctx)
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case callback := <-callbackCh:
			return &callback, nil

		case <-heartbeatTicker.C:
			// 检查 pid 文件
			pidFile := fmt.Sprintf("/tmp/ops-agent-%s.pid", taskID)
			out, _ := execSSHString(sshClient, fmt.Sprintf("test -f %s && echo alive", pidFile))
			if strings.TrimSpace(out) != "alive" {
				// pid 文件不存在，等待 grace period
				select {
				case callback := <-callbackCh:
					return &callback, nil
				case <-time.After(10 * time.Second):
					return nil, fmt.Errorf("Agent 进程已退出但未收到回调")
				case <-ctx.Done():
					return nil, fmt.Errorf("工作流已终止")
				}
			}

		case <-ctx.Done():
			return nil, fmt.Errorf("工作流已终止")
		}
	}
}
```

---

## 17. 实现子工作流机制

在 `executor.go` 中添加子工作流状态管理：

```go
// SubWorkflowState 子工作流运行时状态
type SubWorkflowState struct {
	WorkflowID       string
	ActivationStatus core.ActivationStatus
	Executor         *WorkflowExecutor
	PendingActivate  bool
}

// WorkflowExecutor 中添加字段
// subWorkflows map[string]*SubWorkflowState  // key: node_id

// activateSubWorkflow 激活子工作流
func (e *WorkflowExecutor) activateSubWorkflow(nodeID string, params core.HandleMap) error {
	state, ok := e.subWorkflows[nodeID]
	if !ok {
		return fmt.Errorf("子工作流未找到: %s", nodeID)
	}

	switch state.ActivationStatus {
	case core.ActivationDormant:
		// 加载并启动子工作流
		state.ActivationStatus = core.ActivationActivating
		go func() {
			if err := state.Executor.Run(); err != nil {
				e.logger.Error("子工作流 Ready 失败", zap.Error(err))
				state.ActivationStatus = core.ActivationDormant
			} else {
				state.ActivationStatus = core.ActivationActivated
			}
		}()

	case core.ActivationActivated:
		// 只更新参数
		e.logger.Info("子工作流已激活，更新注入参数", zap.String("node_id", nodeID))

	case core.ActivationRunningOver:
		// 排队
		state.PendingActivate = true
		e.logger.Info("子工作流 Over 执行中，排队激活", zap.String("node_id", nodeID))
	}

	return nil
}
```

---

## 18. 实现 nginx_with_linux 节点

创建 `internal/nodes/nginx/nginx_linux.go`：

```go
package nginx

import (
	"fmt"
	"ops-engine/internal/core"
	"ops-engine/internal/framework"
)

type NginxWithLinuxNode struct{}

func (n *NginxWithLinuxNode) Define() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "nginx_with_linux",
		DisplayName: "Nginx（Linux）",
		Category:    "部署",
		Icon:        "🌐",
		Description: "通过包管理器在 Linux 服务器上安装 Nginx",
		InputPorts: []core.PortDef{
			{ID: "server", Label: "服务器连接", PortType: core.PortTypeLinuxSsh, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "nginx_instance", Label: "Nginx 实例", PortType: core.PortTypeNginxInstance},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "select", ID: "version", Label: "Nginx 版本",
				Options: []string{"latest", "1.26", "1.24", "stable"}},
			{Type: "number", ID: "http_port", Label: "HTTP 端口", Default: 80},
			{Type: "toggle", ID: "start_on_install", Label: "安装后自动启动", Default: true},
		},
		ExecutionMode: core.ExecutionModeAgent,
	}
}

func (n *NginxWithLinuxNode) Validate(config map[string]any) framework.ValidateResult {
	port, _ := config["http_port"].(float64)
	if port <= 0 || port > 65535 {
		return framework.ValidationErr("http_port", "端口范围必须在 1-65535 之间")
	}
	return framework.ValidationOK()
}

func (n *NginxWithLinuxNode) CheckItems(
	config map[string]any,
	inputs framework.HandleMap,
) []framework.CheckItem {
	port := config["http_port"].(float64)
	return []framework.CheckItem{
		{
			Label: "SSH 连接有效", Required: true,
			Action: func() framework.CheckItemResult {
				return framework.CheckPassWith("连接有效")
			},
		},
		{
			Label: fmt.Sprintf("端口 %d 未占用", int(port)), Required: true,
			Action: func() framework.CheckItemResult {
				return framework.CheckPass()
			},
		},
		{
			Label: "磁盘空间充足", Required: true,
			Action: func() framework.CheckItemResult {
				return framework.CheckPass()
			},
		},
	}
}

func (n *NginxWithLinuxNode) Prepare(config map[string]any) []framework.UploadFile {
	return []framework.UploadFile{
		framework.AgentBinaryFile(),
		framework.TaskConfigFile(n.TaskConfig(config)),
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

func (n *NginxWithLinuxNode) OnCallback(result core.AgentCallback) (framework.HandleMap, error) {
	version, _ := result.Output["nginx_version"].(string)
	return framework.HandleMap{
		"nginx_instance": &core.NginxHandle{
			Version:    version,
			ConfigPath: "/etc/nginx/nginx.conf",
		},
	}, nil
}
```

在 `registry/registry.go` 的 `Build()` 中注册：

```go
r.Register(&nginx.NginxWithLinuxNode{})
```

---

## 19. 运行与测试

### 19.1 实现 main.go 入口

创建 `cmd/ops-engine/main.go`：

```go
package main

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"ops-engine/internal/api"
	"ops-engine/internal/registry"
	"ops-engine/internal/store"
)

func main() {
	// 初始化日志
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	zap.L().Info("OpsEngine 启动中...")

	// 确保数据目录存在
	for _, dir := range []string{"data/workflows", "data/executions", "data/logs"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			zap.L().Fatal("创建目录失败", zap.Error(err))
		}
	}

	// 初始化各组件
	reg            := registry.Build()
	workflowStore  := store.NewWorkflowStore("data/workflows")
	executionStore := store.NewExecutionStore("data/executions", "data/logs")
	engineAddr     := "localhost:8080"

	state := &api.AppState{
		Registry:       reg,
		WorkflowStore:  workflowStore,
		ExecutionStore: executionStore,
		Running:        make(map[string]*runtime.WorkflowExecutor),
		EventHub:       api.NewEventHub(),
		EngineAddr:     engineAddr,
	}

	// 启动 HTTP 服务
	router := api.NewRouter(state)
	addr := ":8080"
	zap.L().Info("服务已启动", zap.String("addr", fmt.Sprintf("http://localhost%s", addr)))
	router.Run(addr)
}
```

### 19.2 编译与运行

```powershell
# 整理依赖
go mod tidy

# 编译
go build ./cmd/ops-engine
go build ./cmd/ops-agent

# 运行（开发模式）
go run ./cmd/ops-engine

# 或者用 Makefile
make run
```

### 19.3 创建测试工作流

在 `data/workflows/` 目录下创建 `test_ssh.toml`：

```toml
[workflow]
id          = "test_ssh"
name        = "测试 SSH 连接"
description = "验证 SSH 连接节点是否正常工作"

[[nodes]]
id   = "ready_1"
type = "system_ready"
position = { x = 100, y = 100 }

[[nodes]]
id   = "ssh_conn"
type = "linux_ssh_connection"
position = { x = 400, y = 100 }

[nodes.config]
host     = "192.168.1.10"
port     = 22
username = "root"
password = "your_password"

[[edges]]
from = { node = "ready_1",  port = "signal"  }
to   = { node = "ssh_conn", port = "trigger" }
```

### 19.4 测试接口

```powershell
# 获取所有节点类型
curl http://localhost:8080/api/node-types

# 获取工作流列表
curl http://localhost:8080/api/workflows

# 启动工作流
curl -X POST http://localhost:8080/api/workflows/test_ssh/start

# 查看执行历史
curl http://localhost:8080/api/executions
```

### 19.5 交叉编译 Agent

```powershell
# 编译 Linux amd64
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -o agents/linux_amd64/ops-agent ./cmd/ops-agent

# 编译 Linux arm64
$env:GOARCH = "arm64"
go build -o agents/linux_arm64/ops-agent ./cmd/ops-agent

# 恢复环境变量
Remove-Item Env:GOOS
Remove-Item Env:GOARCH

# 或者用 Makefile（需要安装 make）
make cross-compile
```

---

## 附录：常见 Go 问题

### 编译错误：undefined: xxx
```
原因：忘记 import 包，或者包名写错
解决：VSCode 装了 gopls 会自动提示 import，保存时自动添加
```

### 运行错误：panic: assignment to entry in nil map
```
原因：使用了未初始化的 map
解决：用 make(map[K]V) 初始化
// 错误
var m map[string]int
m["key"] = 1  // panic

// 正确
m := make(map[string]int)
m["key"] = 1
```

### goroutine 泄漏
```
原因：启动了 goroutine 但没有退出机制
解决：始终通过 context 传递取消信号，goroutine 内监听 ctx.Done()

go func() {
    for {
        select {
        case <-ctx.Done():
            return  // 收到取消信号退出
        default:
            // 正常业务逻辑
        }
    }
}()
```

### 并发写 map 导致 panic
```
原因：Go 的 map 不是线程安全的
解决：用 sync.RWMutex 保护，或者改用 sync.Map

type SafeMap struct {
    mu sync.RWMutex
    m  map[string]any
}
```

### JSON 数字默认是 float64
```
原因：Go 的 encoding/json 把 JSON 数字解析为 float64
解决：类型断言时用 float64

// JSON: {"port": 22}
port, _ := config["port"].(float64)  // 不是 int！
portInt := int(port)
```

---

*开发指南版本：v0.1（Go 版）| 对应技术文档：v0.1*
