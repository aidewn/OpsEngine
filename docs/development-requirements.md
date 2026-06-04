# OpsEngine 开发需求文档

## 1. 文档目的

本文基于 OpsEngine 当前已实现能力，梳理后续开发需求、功能边界、用户场景和验收标准。

本文不描述具体代码实现细节，主要用于：

- 明确产品定位；
- 统一后续开发优先级；
- 约束大模型生成工作流功能的范围；
- 避免功能扩展时偏离当前系统架构。

## 2. 当前项目定位

OpsEngine 是一个本地桌面运维工作流编排工具。

核心定位：

```text
通过可视化节点画布，把运维操作流程编排成可复用、可执行、可观测的工作流。
```

当前系统强调：

- 本地运行；
- 本地 TOML 持久化；
- Wails 桌面应用；
- Go 后端执行引擎；
- React Flow 可视化画布；
- 运维环境集中配置；
- 工作流执行状态实时反馈；
- 节点能力可持续扩展。

## 3. 当前已具备能力

### 3.1 工作流管理

当前已支持：

- 创建工作流；
- 查看工作流列表；
- 打开工作流画布；
- 编辑节点和连线；
- 保存工作流；
- 删除工作流；
- 运行工作流；
- 停止运行中的工作流；
- 查看执行记录。

相关后端能力：

- `CreateWorkflow`
- `ListWorkflows`
- `GetWorkflow`
- `UpdateWorkflow`
- `DeleteWorkflow`
- `RunWorkflow`
- `StopWorkflow`
- `ListExecutions`
- `GetExecution`

### 3.2 集合能力

当前已支持集合，也就是可复用子流程。

当前已支持：

- 创建集合；
- 编辑集合画布；
- 定义集合参数；
- 定义集合返回值；
- 在工作流中通过 `assemble:<id>` 调用集合；
- 保存集合时检测循环引用。

集合的价值：

- 复用常见运维步骤；
- 把复杂流程拆成子流程；
- 为后续大模型生成工作流提供可组合能力单元。

### 3.3 执行引擎

当前执行引擎已支持：

- Exec 控制流；
- Data 数据流；
- 节点状态更新；
- 节点日志；
- 工作流变量；
- 集合调用栈 Frame；
- 并行执行；
- 后台线程；
- 条件分支；
- For 循环；
- While 循环；
- Break 中断；
- system ready / update / over 生命周期。

执行状态包括：

```text
Running
Success
Failed
Terminated
```

节点状态包括：

```text
Idle
Executing
Success
Failed
Skipped
Terminated
```

### 3.4 环境配置

当前已支持集中管理运维环境。

环境配置类型包括：

```text
ssh
docker
k8s
jenkins
localhost
registry
```

当前已支持：

- 创建环境；
- 编辑环境；
- 添加环境配置项；
- 测试配置连通性；
- 工作流节点引用环境配置；
- 编辑态探测环境资源。

SSH 配置当前已扩展支持：

- 密码连接；
- 私钥文件连接；
- 私钥口令。

### 3.5 内置运维节点

当前节点能力主要覆盖以下方向。

#### 基础流程节点

```text
system_ready
system_update
system_over
parallel
thread
branch
break
for_loop
while_loop
```

#### 变量和表达式节点

```text
var_get
var_set
arith
compare
logic
to_string
regex_extract
print
```

#### Linux / SSH 节点

```text
env_connect_ssh
ssh_with_linux
linux_exec_command
linux_exec_script
linux_open_file
linux_find_file
linux_upload_file
linux_download_file
linux_file_write
linux_file_append
linux_file_replace
```

#### Docker 节点

```text
env_connect_docker
docker_connect
docker_build
docker_pull
docker_push
docker_run
docker_ps
docker_filter
docker_logs
docker_exec
docker_stop
docker_restart
docker_rm
image_push_tar
```

#### K8s 节点

```text
env_connect_k8s
k8s_connect
k8s_find_workload
k8s_set_workload_image
env_probe_k8s_pods
env_probe_k8s_workloads
```

#### Jenkins / Registry / Localhost 探测节点

```text
env_connect_jenkins
env_connect_localhost
env_probe_jenkins_jobs
env_probe_docker_containers
env_probe_localhost_list_dir
env_probe_localhost_find_files
env_probe_ssh_list_dir
env_probe_ssh_find_files
```

## 4. 目标用户

### 4.1 个人运维人员

目标：

- 把重复操作沉淀成工作流；
- 快速执行服务器巡检、日志采集、容器操作；
- 减少手工 SSH、复制命令、整理报告的成本。

### 4.2 中小团队运维 / DevOps

目标：

- 共享标准化操作流程；
- 对发布、回滚、巡检、排障流程做可视化编排；
- 保留执行记录；
- 降低新人操作门槛。

### 4.3 私有化环境用户

目标：

- 不依赖云厂商 SaaS；
- 环境凭据和执行数据留在本地或内网；
- 接入私有服务器、私有 Docker、私有 K8s、私有大模型。

## 5. 核心业务场景

### 5.1 服务器巡检

用户目标：

```text
对指定服务器执行巡检，采集系统信息、目录、日志、进程或容器状态，并生成巡检结果。
```

典型流程：

```text
连接 SSH
  -> 执行系统命令
  -> 读取关键配置文件
  -> 搜索日志
  -> 汇总输出
```

需要加强的能力：

- 标准巡检模板；
- 巡检报告节点；
- 多服务器批量巡检；
- 巡检结果结构化输出。

### 5.2 故障排查

用户目标：

```text
根据故障描述，自动组合排查步骤，采集日志、指标、容器状态、K8s 状态，并生成排查报告。
```

典型流程：

```text
输入故障现象
  -> 选择环境
  -> 生成排查工作流
  -> 人工确认
  -> 执行采集节点
  -> 汇总排查结果
  -> 生成报告
```

需要加强的能力：

- 大模型生成排查流程；
- 大模型总结执行结果；
- 报告模板；
- 风险动作确认。

### 5.3 容器发布和回滚

用户目标：

```text
把镜像构建、推送、拉取、运行、检查日志、失败回滚编排成标准流程。
```

当前已有基础：

- Docker build；
- Docker pull / push；
- Docker run；
- Docker ps；
- Docker logs；
- Docker stop / restart / rm；
- K8s set workload image。

需要加强的能力：

- 发布模板；
- 镜像版本变量；
- 发布前检查；
- 发布后健康检查；
- 失败分支和回滚节点。

### 5.4 服务器架构解析

用户目标：

```text
读取服务器目录、配置文件、容器、K8s 工作负载，生成系统架构摘要。
```

典型流程：

```text
连接环境
  -> 探测目录
  -> 读取配置
  -> 列 Docker 容器
  -> 列 K8s 工作负载
  -> 交给大模型总结
  -> 输出架构说明
```

需要加强的能力：

- LLM 总结节点；
- 文件内容摘要；
- 拓扑关系结构化输出；
- Markdown 报告导出。

## 6. 近期开发目标

### 6.1 大模型 API 接入

目标：

```text
允许用户配置外部大模型 API，并在后端调用大模型。
```

第一版只支持 OpenAI-compatible API。

新增环境配置：

```text
kind = llm
provider = openai_compatible
base_url
api_key
model
timeout_seconds
```

验收标准：

- 可以在环境配置中新增 LLM 配置；
- 可以测试 LLM 配置是否可用；
- 后端可以调用 `/chat/completions`；
- API key 不写入日志；
- 非 2xx 响应有明确错误信息。

### 6.2 AI 生成工作流

目标：

```text
用户输入自然语言需求，系统调用大模型生成可预览的 WorkflowDef。
```

第一版流程：

```text
用户输入需求
  -> 选择 LLM 配置
  -> 后端收集 node_types / environments / assembles
  -> 大模型输出 JSON
  -> 后端转换为 WorkflowDef
  -> 后端校验
  -> 前端打开画布预览
```

验收标准：

- 能生成最小工作流；
- 能生成包含环境连接节点的工作流；
- 生成结果必须通过后端校验；
- 生成后不自动执行；
- 用户可以在画布中继续编辑。

### 6.3 工作流报告能力

目标：

```text
将工作流执行结果整理成可读报告，支持巡检和排障场景。
```

第一版报告内容：

- 工作流名称；
- 执行时间；
- 执行状态；
- 节点执行结果；
- 关键日志；
- 错误信息；
- 大模型总结。

验收标准：

- 执行详情页可以生成 Markdown 报告；
- 报告可复制；
- 失败节点在报告中突出显示；
- 大模型总结失败时不影响原始执行记录查看。

## 7. 功能需求

### 7.1 工作流编辑

需求：

- 用户可以创建、编辑、保存、删除工作流；
- 用户可以拖拽节点到画布；
- 用户可以连接 Exec 和 Data 端口；
- 用户可以配置节点参数；
- 用户可以通过变量复用数据；
- 用户可以复制粘贴节点；
- 用户可以删除节点和连线。

约束：

- 保存时必须通过后端校验；
- 不允许非法端口连接；
- 不允许未知节点类型；
- 不允许数据输入端口多入边；
- 不允许 exec 输出端口多出边。

### 7.2 集合复用

需求：

- 用户可以创建集合；
- 用户可以配置 Params 和 Returns；
- 工作流可以调用集合；
- 集合可以调用其他集合。

约束：

- 集合不能直接或间接引用自身；
- 集合内部必须有合法入口和出口；
- 集合调用时参数和返回端口应动态生成。

### 7.3 环境管理

需求：

- 用户可以创建环境；
- 环境内可以配置 SSH、Docker、K8s、Jenkins、Localhost、Registry；
- 后续需要支持 LLM；
- 环境配置可以被工作流节点引用；
- 用户可以测试配置连通性。

约束：

- 敏感信息不应出现在执行日志中；
- SSH 私钥只保存路径，不保存私钥内容；
- Docker over SSH 应复用 SSH 配置；
- 配置 ID 必须唯一。

### 7.4 执行和观测

需求：

- 用户可以运行工作流；
- 用户可以停止工作流；
- 用户可以查看执行列表；
- 用户可以查看执行详情；
- 用户可以查看节点日志；
- 用户可以查看集合调用栈；
- 用户可以查看变量变化。

约束：

- 执行中修改工作流不影响当前运行快照；
- Stop 后未完成节点应标记为 Terminated；
- 执行记录应可持久化；
- 后台线程和并行分支应被正确收尾。

### 7.5 大模型生成工作流

需求：

- 用户可以输入自然语言需求；
- 用户可以选择 LLM 配置；
- 系统可以把当前可用节点、环境、集合提供给模型；
- 模型返回结构化 JSON；
- 系统转换为 WorkflowDef；
- 系统校验生成结果；
- 前端展示生成结果。

约束：

- 模型不能直接写 TOML；
- 模型不能直接执行工作流；
- 模型不能编造未知节点；
- 模型不能编造环境配置 ID；
- 模型输出必须经过后端校验；
- 危险节点需要用户确认。

## 8. 非功能需求

### 8.1 安全

- 敏感字段默认不出现在日志和报告中；
- API key、密码、私钥口令应避免明文展示；
- 私钥内容不进入配置文件；
- 大模型调用时不发送密码、token、私钥路径等敏感字段；
- 危险操作节点不得自动执行。

### 8.2 稳定性

- 工作流执行失败应保留失败原因；
- 单节点失败不应导致 UI 白屏；
- 大模型调用失败不应影响已有工作流编辑和执行；
- 前端 dev server 应固定 IPv4，避免 Windows localhost IPv6 问题。

### 8.3 可扩展性

- 新节点应通过 `internal/nodes` 注册；
- 新环境类型应通过 `EnvConfigKind` 扩展；
- 新客户端能力应放在 `internal/clients`；
- 大模型 Provider 应可扩展，不绑定单一厂商。

### 8.4 可观测性

- 执行状态实时更新；
- 节点日志可追踪；
- 集合调用栈可定位；
- 生成工作流失败时应暴露可读错误。

## 9. 里程碑规划

### M1：稳定现有基础能力

范围：

- 修复启动和开发环境问题；
- 确保 `go test ./...` 通过；
- 确保 `npm run lint` 通过；
- 确认环境配置、执行、画布保存稳定。

验收：

```text
wails dev 可以正常启动
go test ./... 通过
npm run lint 通过
已有工作流可以打开、保存、运行
```

### M2：接入外部大模型 API

范围：

- 新增 LLM 环境类型；
- 新增 LLM 客户端；
- 新增 LLM 连通性测试；
- 支持 OpenAI-compatible API。

验收：

```text
可以配置模型服务
可以测试模型服务
后端可以获得模型返回文本
```

### M3：AI 生成工作流

范围：

- 新增 GenerateWorkflow 后端接口；
- 新增 AI 生成弹窗；
- 大模型输出 JSON plan；
- 后端转换并校验 WorkflowDef；
- 画布预览生成结果。

验收：

```text
输入“生成一个打印 hello 的工作流”
系统生成 system_ready -> print
用户可以保存并手动运行
```

### M4：巡检和排障模板

范围：

- 内置服务器巡检生成模板；
- 内置 Docker 排障模板；
- 内置 K8s 工作负载排障模板；
- 支持报告生成。

验收：

```text
可以生成服务器巡检工作流
可以执行并得到 Markdown 巡检报告
```

### M5：报告和知识沉淀

范围：

- 执行记录生成报告；
- 大模型总结执行结果；
- 报告复制或导出；
- 常用模板沉淀为集合。

验收：

```text
执行详情页可以生成排查报告
报告包含失败节点、关键日志和建议动作
```

## 10. 第一版明确不做

第一版不做：

- MCP；
- 流式 token 输出；
- 模型自动执行工作流；
- 自动修改线上环境；
- 复杂权限系统；
- 多用户协作；
- 云端工作流同步；
- 插件市场；
- 完整密钥保险箱。

## 11. 风险和控制措施

### 11.1 模型生成错误工作流

控制：

- 后端强校验；
- 只允许已注册节点；
- 生成后只预览；
- 用户手动确认保存和运行。

### 11.2 模型生成危险操作

控制：

- 危险节点清单；
- 默认不自动执行；
- UI 显示风险提示；
- 后续增加审批确认。

### 11.3 敏感信息泄露

控制：

- Prompt 中只发送环境摘要；
- 不发送密码、token、私钥路径；
- 执行报告默认脱敏；
- 日志禁止输出敏感字段。

### 11.4 外部模型服务不可用

控制：

- 明确超时；
- 失败提示；
- 不影响现有工作流能力；
- 支持切换 LLM 配置。

## 12. 验收总标准

阶段性完成后，系统应满足：

```text
用户能配置服务器、Docker、K8s 等环境；
用户能手工编排和执行运维工作流；
用户能调用外部大模型生成工作流草稿；
生成结果经过后端校验；
生成结果可以在画布中人工确认和修改；
执行结果可以形成基础报告；
危险操作不会被模型自动执行。
```

