# OpsEngine

面向运维场景的可视化工作流桌面应用。通过节点图画出执行流与数据流，在本地运行并实时查看状态与日志；支持将子流程封装为可复用的**集合**（Assemble），在工作流或其它集合中调用。

## 功能概览

- **工作流编辑**：基于 React Flow 的画布，拖拽节点、连线，自动保存位置
- **集合（子流程）**：参数 / 返回值端口，保存时检测循环引用
- **三阶段生命周期**：`system_ready`（启动）→ `system_update`（周期/手动增量，可选）→ `system_over`（收尾）
- **流程控制**：并行（`parallel`）、后台线程（`thread`）、中断（`break`）、停止执行
- **本地执行引擎**：Exec / Data 双流调度，集合调用栈以 Frame 树记录状态与日志
- **实时反馈**：通过 Wails 事件推送节点状态、日志、变量变更
- **持久化**：工作流、集合、终态执行记录以 TOML 保存在 `data/` 目录

> 业务向节点（SSH、Docker、K8s 等）端口类型已在模型中预留；当前内置节点以流程与示例（如 `print`）为主，便于扩展。

## 技术栈

| 层级 | 技术 |
|------|------|
| 桌面壳 | [Wails v2](https://wails.io)（Go + WebView） |
| 后端 | Go 1.25、uber/zap、BurntSushi/toml |
| 前端 | React 18、TypeScript、Vite、@xyflow/react、Tailwind CSS |
| 通信 | Wails 方法绑定 + 运行时事件（无独立 HTTP 服务） |

## 环境要求

- **Go** 1.25+
- **Node.js** 18+ 与 npm
- **Wails CLI** v2

  ```bash
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
  ```

- **Windows**：需安装 [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)（Win10/11 通常已自带）
- **Linux / macOS**：按 [Wails 官方文档](https://wails.io/docs/gettingstarted/installation) 安装对应系统依赖

## 快速开始

```bash
git clone https://github.com/<your-org>/OpsEngine.git
cd OpsEngine

# 开发模式（热重载：Go 后端 + 前端）
make dev
# 或
wails dev
```

首次启动会自动创建数据目录：

```
data/
├── workflows/    # 工作流定义（*.toml）
├── assembles/    # 集合定义（*.toml）
├── executions/   # 终态执行记录（*.toml，已在 .gitignore）
└── logs/         # 运行日志（已在 .gitignore）
```

### 构建发布版

```bash
make build
# 产物位于 build/bin/
```

### 运行测试

```bash
make test
# 或
go test ./...
```

引擎相关测试集中在 `internal/engine/`。

## 使用说明

1. 启动应用后，在首页 **工作流** 标签创建并打开工作流。
2. 画布上编辑节点与连线；右侧可查看节点详情与工作流变量。
3. 在 **集合** 标签维护可复用子图；保存后会在工作流画布的「添加节点」中作为 `assemble:<id>` 出现。
4. 运行工作流后，在 **执行** 标签或执行详情页查看实时状态与历史记录。

### 端口与连线规则（摘要）

| 类型 | 约束 |
|------|------|
| `exec_out` | 单出（一条出边） |
| `exec_in` | 单入 |
| 数据 output | 多出 |
| 数据 input | 单入 |

保存工作流 / 集合时，后端会校验结构合法性；画布连接时支持拖到已有入边端口自动替换旧连接。

## 架构

```
┌─────────────────────────────────────────────────────────┐
│  React 前端（frontend/src）                              │
│  画布 · 执行监控 · Wails JS 绑定                         │
└───────────────────────────┬─────────────────────────────┘
                            │ Bind + Events
┌───────────────────────────▼─────────────────────────────┐
│  app.go（Wails 入口：CRUD、RunWorkflow、GetNodeTypes）   │
└───────────────────────────┬─────────────────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        ▼                   ▼                   ▼
  internal/store      internal/engine     internal/nodes
  TOML 持久化          执行 / 调度 / 校验    内置节点注册
```

**执行事件名**（前后端约定，见 `internal/engine/events.go`）：

- `execution:started` / `execution:status` / `execution:finished`
- `execution:node` / `execution:log` / `execution:variable`

事件 payload 可含 `framePath`，用于定位集合调用栈内的节点状态。

## 目录结构

```
OpsEngine/
├── main.go                 # Wails 应用入口
├── app.go                  # 暴露给前端的 API
├── Makefile
├── wails.json
├── docs/
│   └── phase8-13-plan.md   # 后续迭代计划
├── data/                   # 本地数据（部分目录不入库）
├── frontend/               # React 前端
│   └── src/
│       ├── pages/          # 路由页面
│       ├── features/       # workflow / assemble / execution
│       ├── api/            # Wails 调用封装
│       └── types/          # 与 Go 结构对齐的 TS 类型
└── internal/
    ├── core/               # 领域模型
    ├── engine/             # 执行引擎
    ├── nodes/              # 内置节点（init 注册）
    └── store/              # TOML 存储
```

## 开发指南

### 新增内置节点

1. 在 `internal/nodes/<name>/` 实现 `engine.Node`（`TypeDef` + `Execute`）。
2. 在包内 `init()` 中调用 `engine.Register`。
3. 在 `internal/nodes/nodes.go` 增加匿名 import，触发注册。

### 前后端协作

- **绑定方法**：`app.go` 中 `App` 的 public 方法自动生成前端调用（`wails dev` 后出现在 `frontend/wailsjs/go/main/App`）。
- **类型**：Go 的 `json` tag 与 `frontend/src/types/` 保持一致（snake_case）。
- **集合节点类型**：运行时由 `GetNodeTypes()` 将每个 `AssembleDef` 转为 `assemble:<id>` 节点类型。

### 常用命令

| 命令 | 说明 |
|------|------|
| `make dev` | 开发模式 |
| `make build` | 构建桌面应用 |
| `make test` | 运行 Go 测试 |
| `make fmt` | `go fmt ./...` |
| `make tidy` | `go mod tidy` |

仅调试前端 UI 时（需已 `wails dev` 或自行处理绑定）：

```bash
cd frontend
npm install
npm run dev
```

## 路线图

MVP（Phase 0–7）已完成。后续体验与能力见 [docs/phase8-13-plan.md](docs/phase8-13-plan.md)，包括端口重连、配置表单、Frame 树 UI、框选复制等。

## 参与贡献

欢迎 Issue 与 Pull Request。提交前请：

1. 运行 `make test` 确保通过。
2. 遵循仓库内 `CLAUDE.md` 的约定（注释使用中文、改动保持精简）。
3. 不提交 `data/executions`、`data/logs` 等本地运行数据。

## 许可证

本项目采用 [MIT License](LICENSE) 开源。
