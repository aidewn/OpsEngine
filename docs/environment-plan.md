# 配置环境与环境探测节点 — 实现计划

> 主页「配置环境」+ 按业务/项目集中管理 SSH/Docker/K8s/Jenkins 凭证；  
> 独立探测节点在编辑态「探测一次」点选结果，运行态经**数据端口连线**传给下游（主路径）；  
> 写入 workflow 变量为可选。与 [architecture.md](./architecture.md)、[node-development.md](./node-development.md) 配合阅读。

---

## 0. 总览

| 阶段 | 主题 | 状态 |
|------|------|------|
| Phase 1 | Environment CRUD + 主页 Tab + 配置表单 + TestEnvConfig | 待做 |
| Phase 2 | `env_connect_ssh` + `env_probe_ssh_list_dir` + Probe API + Picker UI | 待做 |
| Phase 3 | `env_probe_ssh_find_files` + Docker connect/probe | 待做 |
| Phase 4 | K8s connect + `env_probe_k8s_pods` | 待做 |
| Phase 5 | Jenkins 配置 + connect + probe | 待做 |
| Phase 6 | static/dynamic 完善、assemble 引用环境（可选） | 待做 |

**已定设计决策（不再讨论）：**

| 项 | 结论 |
|----|------|
| 环境粒度 | 按**业务/项目**（如「nginx 生产」「yum 测试环境」） |
| 凭证存储 | MVP **明文** TOML（与现有 `ssh_with_linux` 一致） |
| 探测结果持久化 | 必写节点 `probe_snapshot`；`workflow.variables` **可选**（勾选同步） |
| 运行时传值 | **主路径：数据边**（探测 output → 下游 input）；不依赖运行前注入 pass |
| 运行模式 | 节点 config `resolve_mode`：`static` \| `dynamic`，**默认 static** |
| 探测节点形态 | **每种探测独立 TypeID**；仅环境配置共享 |
| 探测节点 exec | **无 exec 端口** — Pure 数据源，主线 exec 流不经过 |
| 变量绑定 | **可选** `variable_bindings`，支持多条（多处复用 / 仅 config 框场景） |
| 旧节点 | **保留**（如 `linux_find_file`）— 运行时再分析文件等场景仍用 exec 链节点 |

---

## 1. 背景与目标

### 1.1 现状痛点

- SSH 密码散落在 `ssh_with_linux` 节点 config，多工作流重复配置。
- 想知道远程目录有哪些文件 → 需先**跑整条工作流** → 从日志抄路径 → 再填 config。
- Docker/K8s 连接与 `internal/clients` 已有能力未与编辑态 UX 打通。

### 1.2 目标

1. 主页增加 **「配置环境」** Tab：按项目创建环境，环境内添加多类型配置。
2. **连接节点**（有 exec）：从环境解析凭证 → 输出运行时句柄（SSH/Docker/K8s）。
3. **探测节点**（无 exec）：编辑时探测 + 点选 → 写 `probe_snapshot`；画布上用**数据边**连到下游 input；运行时可 static 或 dynamic。
4. 与现有节点**并存**：运行时动态找文件仍用 `linux_find_file` 等 Action 节点。

---

## 2. 架构分层

```
┌─────────────────────────────────────────────────────────┐
│  主页 · 配置环境 Tab                                      │
│  Environment CRUD · 配置卡片 · TestEnvConfig              │
└──────────────────────────┬──────────────────────────────┘
                           │ 引用 environment_id + config_id
┌──────────────────────────▼──────────────────────────────┐
│  工作流画布                                               │
│                                                          │
│  exec 主线:  system_ready → env_connect_ssh → … → action │
│                                                          │
│  数据边（主路径）:                                          │
│    env_probe_ssh_list_dir.selected_path                  │
│         ──────────────────────────→ linux_open_file.path │
│                                                          │
│  env_probe_* : Pure · 无 exec · 编辑态 Probe → snapshot  │
│  env_connect_* : Action · 有 exec · 输出 SSH/Docker 句柄   │
│                                                          │
│  linux_find_file 等旧节点保留（运行时动态探测，走 exec 链）  │
└─────────────────────────────────────────────────────────┘
```

### 2.1 数据流求值（与现有引擎一致）

探测节点为 **Pure** 时，下游需要某 input 会触发 `evaluator.evalInput` → `evalOutput` → 探测节点 `Execute`（见 `internal/engine/evaluator.go`）。

- **不需要**单独的「运行前变量注入 pass」。
- **static**：`Execute` 不连远程，从 `probe_snapshot` 映射到各 output port 返回值。
- **dynamic**：`Execute` 执行与编辑态 `ProbeEnvNode` 相同的远程逻辑，返回实时结果。
- 无人引用的探测节点（无出边、无 variables 同步）运行时**不会**被求值，无副作用。

---

## 3. 数据模型

### 3.1 持久化

- 路径：`data/environments/{id}.toml`
- Store：`internal/store/environment_store.go`（对齐 workflow/assemble CRUD）

### 3.2 Go 结构（`internal/core/environment.go`）

```go
// EnvironmentDef 业务/项目环境
type EnvironmentDef struct {
    ID          string          `json:"id"          toml:"id"`
    Name        string          `json:"name"        toml:"name"`
    Description string          `json:"description" toml:"description"`
    Configs     []EnvConfigItem `json:"configs"     toml:"configs"`
}

// EnvConfigKind 配置类型
type EnvConfigKind string

const (
    EnvConfigKindSSH      EnvConfigKind = "ssh"
    EnvConfigKindDocker   EnvConfigKind = "docker"
    EnvConfigKindK8s      EnvConfigKind = "k8s"
    EnvConfigKindJenkins  EnvConfigKind = "jenkins"
)

// EnvConfigItem 环境内单条配置（fields 按 kind 解析）
type EnvConfigItem struct {
    ID          string            `json:"id"          toml:"id"`
    Name        string            `json:"name"        toml:"name"`
    Kind        EnvConfigKind     `json:"kind"        toml:"kind"`
    Description string            `json:"description" toml:"description"`
    Fields      map[string]any    `json:"fields"      toml:"fields"`
}
```

### 3.3 各 kind 的 fields 约定

| kind | fields | 说明 |
|------|--------|------|
| **ssh** | `host`, `port`, `user`, `password`, `timeout_seconds` | → `clients` SSH |
| **docker** | `mode`: `over_ssh` \| `tcp` | |
| | over_ssh: `ssh_config_id`, `socket_path`（默认 `/var/run/docker.sock`） | 引用**同环境**内 ssh 配置 id |
| | tcp: `host`, `tls_*`（后期） | |
| **k8s** | `kubeconfig_yaml`, `context`, `namespace` | → `NewK8sClientFromKubeconfig` |
| **jenkins** | `base_url`, `user`, `api_token` | Phase 5 新建 client |

示例环境「nginx 生产」：

```
configs:
  - id: ssh-app      kind: ssh     name: 应用机
  - id: docker-app   kind: docker  refs: ssh-app
  - id: k8s-prod     kind: k8s
  - id: jenkins-ci   kind: jenkins
```

---

## 4. 主页 UI

### 4.1 Tab

`HomePage` 增加第四个 Tab：**配置环境**（与 工作流 / 集合 / 执行 并列）。

### 4.2 页面

| 路由 | 组件 | 行为 |
|------|------|------|
| `/` tab | `EnvironmentList` | 列表、创建、删除 |
| `/environments/:id` | `EnvironmentDetailPage` | 编辑环境名、管理 configs |

### 4.3 环境详情

- 「+ 添加配置」→ 选择 kind → 动态表单
- 每条配置：**编辑** / **删除** / **测试连接**（`TestEnvConfig`）
- 密码/token 字段脱敏展示；编辑时用 password 输入框

---

## 5. 后端 API（Wails · `app.go`）

```go
// 环境 CRUD
ListEnvironments() ([]EnvironmentSummary, error)
GetEnvironment(id string) (EnvironmentDef, error)
CreateEnvironment(name, description string) (string, error)
UpdateEnvironment(env EnvironmentDef) error
DeleteEnvironment(id string) error

// 环境详情 · 测试连接（不写 execution 记录）
TestEnvConfig(envID, configID string) error

// 编辑态探测（不写 execution 记录；按节点 TypeID 分发）
ProbeEnvNode(req ProbeNodeRequest) (ProbeNodeResult, error)
```

```go
type ProbeNodeRequest struct {
    TypeID        string         // 如 env_probe_ssh_list_dir
    EnvID         string
    ConfigID      string
    NodeConfig    map[string]any // 该探测节点特有参数 path/pattern 等
}

type ProbeNodeResult struct {
    Items []ProbeItem // { key, label, meta }
}

type ProbeItem struct {
    Key   string
    Label string
    Meta  map[string]any
}
```

探测实现：`internal/probe/` 分包 + 注册表，各节点 TypeID 注册对应函数；复用 `internal/clients`。

---

## 6. 节点族谱

### 6.1 连接类（Action · 有 exec · 输出句柄）

| TypeID | 引用 kind | 输出 port | NodeKind |
|--------|-----------|-----------|----------|
| `env_connect_ssh` | ssh | `client` · LinuxSshConnection | action |
| `env_connect_docker` | docker | `client` · DockerContext | action |
| `env_connect_k8s` | k8s | `client` · K8sContext | action |
| `env_connect_jenkins` | jenkins | `client` · JenkinsContext（新 PortType，P5） | action |

共有 ConfigSchema：

- `environment_id`（select，来自 ListEnvironments）
- `config_id`（select，过滤 kind）

### 6.2 探测类（Pure · **无 exec** · 变量源）

| TypeID | 引用 kind | 能力 | 特有 config | 典型输出 port |
|--------|-----------|------|-------------|---------------|
| `env_probe_ssh_list_dir` | ssh | SFTP 列目录 | `path`, `include_files`, `include_dirs` | `selected_path`, `paths`, `count` |
| `env_probe_ssh_find_files` | ssh | 正则搜文件 | `pattern`, `start_dir`, `max_depth` | `selected_path`, `paths`, `first_path` |
| `env_probe_docker_containers` | docker | 容器列表 | `all`, `filter_name` | `selected_id`, `selected_name`, `names` |
| `env_probe_k8s_pods` | k8s | Pod 列表 | `namespace`, `label_selector` | `selected_pod`, `selected_namespace`, `names` |
| `env_probe_jenkins_jobs` | jenkins | Job 列表 | `folder` | `selected_job` |

**NodeKind：`pure`** — 无 `exec_in` / `exec_out`；画布上仅作为数据源，不参与 exec 主链。

**引擎**：沿用现有 Pure 按需求值；**下游有数据边连入 input 时**即会触发探测节点 `Execute`，无需改 evaluator 主流程。

---

## 7. 探测节点统一契约

### 7.1 共有 Config 字段（各探测节点 ConfigSchema 合并）

| 字段 | 类型 | 说明 |
|------|------|------|
| `environment_id` | select | 环境 ID |
| `config_id` | select | 该环境内匹配 kind 的配置 ID |
| `resolve_mode` | select | `static`（默认）\| `dynamic` |
| `probe_snapshot` | object（只读，UI 写） | `{ picked_key, picked_label, captured_at }` |
| `variable_bindings` | array（**可选**） | 见 §7.4；勾选「同步到工作流变量」时使用 |

`variable_bindings` 每项（可选）：

```json
{
  "variable_name": "target_path",
  "output_port": "selected_path"
}
```

支持**多条** binding（一次探测写多个变量）。

### 7.2 编辑态 UX（Config 面板）

1. 填 `environment_id`、`config_id` 及节点特有参数（如 `path`）。
2. 点击 **「探测一次」** → `ProbeEnvNode` → 展示可选列表。
3. 用户点选一项 → 点击 **「应用」**：
   - **必做**：写 `node.config.probe_snapshot`（含 `picked_key`、`picked_label`、`captured_at`）。
   - **推荐**：提示用户从探测节点的 output 手柄**拉数据边**到下游 input（如 `selected_path` → `linux_open_file` 的 `path`）。
   - **可选**：若勾选「同步到工作流变量」，按 `variable_bindings` 更新 `workflow.variables[].default`（变量须已存在；Phase 2 可不做自动创建）。
4. `update.mutate(workflow)` 持久化。

Phase 2+ 可增强：应用后自动建议连线（高亮可连端口）。

### 7.3 运行态语义（端口直连为主）

| resolve_mode | 行为 |
|--------------|------|
| **static**（默认） | 不连远程。`Execute` 根据 `probe_snapshot` 填充各 **output port** 返回值；下游经**数据边** `evalInput` 取得。 |
| **dynamic** | `Execute` 执行真实探测（list / ps / pods 等），output port 为实时结果。 |

**与 exec 主链的关系：**

```
system_ready ──exec──► env_connect_ssh ──exec──► linux_open_file ──exec──► …

env_probe_ssh_list_dir.selected_path ──data──► linux_open_file.path
                      （Pure，不在 exec 链上）
```

路径/容器名等由**数据边**传入下游；主线只负责 SSH 连接与文件操作等 Action。

**无需**运行前扫描全图、向 `frame.Variables` 注入的专用 pass。

### 7.4 传值方式对比

| 方式 | 何时用 | 运行时机制 |
|------|--------|------------|
| **数据边（主路径）** | 下游节点有对应 input port | `evalOutput` → 探测 Pure `Execute` → 边传递 |
| **workflow.variables（可选）** | 多处复用、或仅 config 框无 port、或左侧变量面板可见性 | 用户勾选同步；下游用 `var_get` 或 config 引用变量名 |
| **节点 config 手填** | 不探测、临时覆盖 | 与现有一致；port 未连线时用 config 默认值 |

「应用」时最小持久化：

```
node.config.probe_snapshot ← 选中项（static 运行必需）
```

可选同步（UI 勾选）：

```
foreach binding in variable_bindings:
  workflow.variables[name].default ← 对应 output 值
```

运行时 **dynamic** 且勾选 variables 同步时：可在 `Execute` 后按 binding 调用 `SetVariable` 更新 **runtime** frame（不改 TOML，除非用户再次「应用」）；**仅数据边、不勾选同步** 时不必写 variables。

---

## 8. 与旧节点的关系

| 场景 | 推荐 |
|------|------|
| 编辑时确定固定配置文件路径 | `env_probe_ssh_list_dir` + static + **数据边**连下游 path |
| 运行中按条件搜索最新 log | `linux_find_file`（Action，走 exec 链） |
| 临时 SSH、不入环境库 | `ssh_with_linux`（保留） |
| 集合 param 传 SSH | `assemble_param` + 工作流侧 `env_connect_ssh`（P6 可改为传 env 引用） |

**不 deprecate 旧节点**；文档与 AddNodeDialog 中区分「环境探测（设计态）」与「运行时远程操作」。

---

## 9. 前端文件清单（预估）

| 文件 | 职责 |
|------|------|
| `frontend/src/pages/HomePage.tsx` | 增加 environment tab |
| `frontend/src/pages/EnvironmentDetailPage.tsx` | 环境详情 |
| `frontend/src/features/environment/EnvironmentList.tsx` | 列表 |
| `frontend/src/features/environment/EnvConfigForm.tsx` | 按 kind 动态表单 |
| `frontend/src/api/environments.ts` | TanStack Query + Wails |
| `frontend/src/features/workflow/EnvProbePanel.tsx` | 探测 + Picker + 应用 snapshot + 可选 variables 同步 |
| `frontend/src/types/environment.ts` | TS 类型 |
| `frontend/src/types/nodeType.ts` | 新 PortType JenkinsContext（P5） |

路由：在 `App.tsx` / router 注册 `/environments/:id`。

---

## 10. 后端文件清单（预估）

| 文件 | 职责 |
|------|------|
| `internal/core/environment.go` | 领域模型 |
| `internal/store/environment_store.go` | TOML CRUD |
| `internal/probe/registry.go` | TypeID → ProbeFunc |
| `internal/probe/ssh_list_dir.go` | … | 
| `internal/probe/docker_containers.go` | … |
| `internal/nodes/env_connect_ssh/` | 连接节点 |
| `internal/nodes/env_probe_ssh_list_dir/` | 探测节点 |
| `app.go` | Wails 绑定 + startup 创建 `data/environments` |

---

## 11. 分阶段验收

### Phase 1

- [ ] 主页「配置环境」Tab 可见
- [ ] 创建环境、添加 SSH 配置、保存 TOML
- [ ] TestEnvConfig 对 SSH 成功/失败有明确提示
- [ ] `go test ./internal/store/...` 含 environment_store 测试

### Phase 2

- [ ] `env_connect_ssh` 运行时可输出 LinuxSshConnection
- [ ] `env_probe_ssh_list_dir` 无 exec 端口，画布显示为 Pure
- [ ] Config 面板「探测一次」→ 列表 → 「应用」→ 写入 `probe_snapshot`
- [ ] 画布：`selected_path` 数据边连到下游 input；static 运行后下游拿到快照路径
- [ ] dynamic 模式：有边引用时重新 list，下游拿到最新路径
- [ ] （可选）勾选 variables 同步后，`var_get` 能读到 default

### Phase 3–5

- [ ] 各 connect/probe 节点与 clients 对齐
- [ ] Docker over_ssh 引用同环境 ssh config_id

---

## 12. Agent 分批建议

| 批次 | 内容 |
|------|------|
| Agent 1 | Phase 1 全量 |
| Agent 2 | Phase 2（env_connect_ssh + env_probe_ssh_list_dir + EnvProbePanel + 数据边联调） |
| Agent 3 | Phase 3 Docker |
| Agent 4 | Phase 4 K8s |
| Agent 5 | Phase 5 Jenkins + Phase 6 可选 |

---

## 13. 风险与边界

- **凭证明文**：仅适合本机桌面；后续可加 OS keychain / `${ENV:}`。
- **static 快照过期**：服务器上路径已删仍用旧值 — UI 可提示 `captured_at`，支持重新探测。
- **Pure 探测节点不在 exec 链**：用户需用**数据边**把探测结果接到下游 input；仅写 snapshot 不连线则下游读不到（符合 Pure 语义）。
- **仅 config 无 port**：若下游只支持 config 框、未连 input，需手填 config、或勾选 variables 同步 + `var_get`。
- **多 binding 类型**：MVP 探测输出均为 String（`paths` 等列表可 JSON 字符串）；复杂类型后续扩展 binding 类型校验。

---

## 14. 参考实现

- SSH 连接：`internal/nodes/ssh_with_linux/node.go`
- Docker over SSH：`internal/clients/docker.go`
- K8s：`internal/clients/k8s.go`
- SFTP 列目录：可复用 `linux_find_file` 的 Walk 逻辑 → 抽到 `internal/clients` 或 `internal/probe`
- 变量面板：`frontend/src/features/workflow/VariablePanel.tsx`
- Store 模式：`internal/store/workflow_store.go`
- Pure 求值：`internal/engine/evaluator.go`（`evalInput` / `evalOutput`）
