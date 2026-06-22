# OpsEngine 多监控源接入开发方案

## 1. 背景

当前监控体系已经具备环境、分组、监控项、批量采集、Monitor Flow 判断、Incident 与报告闭环的基础形态。现有 `DataRequirement` 主要面向环境内置配置，例如 SSH、Docker、K8s、HTTP 探活等。

真实运维环境中，大部分企业已经有 Prometheus、Grafana Loki、Elasticsearch、云监控等系统。OpsEngine 不应该重复建设完整监控平台，而应该接入这些外部系统，将自己定位为：

> 关键监控项的轻量判断层 + 异常诊断与 AI 排查闭环。

因此需要在监控模块中新增“监控源”管理能力，让用户可以在环境下配置 Prometheus 等外部数据源，并让监控项通过统一的数据需求引用这些监控源。

## 2. 目标

第一阶段目标：

- 支持环境级监控源管理。
- 支持 Prometheus 作为第一种外部监控源。
- 监控项可以选择监控源、配置查询参数、配置阈值条件。
- 采集阶段按 `source_id + kind + params` 汇总去重。
- 正常采集数据仍然不落盘，只保存状态、异常事件和诊断报告。
- 保持内置采集源可用，不破坏当前 SSH/Docker/K8s/HTTP 监控能力。

非目标：

- 不内置时序数据库。
- 不保存 Prometheus 查询结果历史。
- 不做 Grafana 式完整 Dashboard。
- 不做 Prometheus Alertmanager 替代品。
- 不在第一阶段支持所有外部监控系统。

## 3. 产品形态

监控的信息架构调整为：

```text
监控
  -> 环境
      -> 概览
      -> 监控源
      -> 分组
          -> 监控项
      -> 异常历史
      -> 诊断报告
```

环境级页面增加二级入口：

```text
[概览] [监控源] [分组] [异常历史]
```

“监控源”页面用于管理当前环境可用的数据来源。示例：

| 名称 | 类型 | 状态 | 地址 / 目标 |
| --- | --- | --- | --- |
| 内置采集 | builtin | 启用 | 当前环境配置 |
| 生产 Prometheus | prometheus | 启用 | `http://prometheus:9090` |
| 日志 Loki | loki | 禁用 | `http://loki:3100` |

第一阶段只实现：

- 内置采集源展示。
- Prometheus 新增、编辑、删除、启用、禁用、测试连接。

## 4. 领域模型

### 4.1 MonitorSource

新增环境级监控源实体：

```go
type MonitorSource struct {
    ID            string         `json:"id"             toml:"id"`
    EnvironmentID string         `json:"environment_id" toml:"environment_id"`
    Name          string         `json:"name"           toml:"name"`
    Kind          string         `json:"kind"           toml:"kind"`
    Enabled       bool           `json:"enabled"        toml:"enabled"`
    Config        map[string]any `json:"config"         toml:"config"`
    CreatedAt     time.Time      `json:"created_at"     toml:"created_at"`
    UpdatedAt     time.Time      `json:"updated_at"     toml:"updated_at"`
}
```

`Kind` 第一阶段取值：

| Kind | 含义 |
| --- | --- |
| `builtin` | 环境内置采集源，桥接现有 SSH/Docker/K8s/HTTP 能力 |
| `prometheus` | Prometheus HTTP API |

后续可扩展：

| Kind | 含义 |
| --- | --- |
| `loki` | Grafana Loki 日志查询 |
| `elasticsearch` | Elasticsearch / OpenSearch |
| `cloud_monitor` | 云厂商监控 API |
| `custom_http` | 自定义 HTTP JSON API |

### 4.2 Prometheus 配置

Prometheus 源配置示例：

```json
{
  "endpoint": "http://prometheus:9090",
  "auth_type": "none",
  "username": "",
  "password": "",
  "token": "",
  "timeout_seconds": 10
}
```

认证方式：

| auth_type | 字段 |
| --- | --- |
| `none` | 无 |
| `basic` | `username`、`password` |
| `bearer` | `token` |

敏感字段：

- `password`
- `token`
- 后续可能新增的 `headers.Authorization`

这些字段不能在普通查询接口中明文返回前端。编辑时可采用“已配置但不回显”的方式。

### 4.3 DataRequirement 升级

当前模型：

```go
type DataRequirement struct {
    TargetID string         `json:"target_id" toml:"target_id"`
    Kind     string         `json:"kind"      toml:"kind"`
    Params   map[string]any `json:"params"    toml:"params"`
}
```

建议升级为：

```go
type DataRequirement struct {
    SourceID string         `json:"source_id" toml:"source_id"`
    TargetID string         `json:"target_id,omitempty" toml:"target_id,omitempty"`
    Kind     string         `json:"kind"      toml:"kind"`
    Params   map[string]any `json:"params"    toml:"params"`
}
```

字段含义：

| 字段 | 含义 |
| --- | --- |
| `source_id` | 监控源 ID，表示从哪里取数据 |
| `target_id` | 内置采集源下的环境配置 ID，兼容 SSH/Docker/K8s 等目标 |
| `kind` | 数据类型或查询能力 |
| `params` | 查询参数 |

兼容策略：

- 存量数据没有 `source_id` 时，视为引用环境默认内置源。
- `target_id` 保留，专门用于 builtin 源内部选择 SSH/Docker/K8s 等环境配置。
- 新增 Prometheus 监控项必须使用 `source_id`。

### 4.4 MonitorCondition 升级

当前条件通过 `kind + target_id + field` 匹配采集结果。引入多源后，需要避免不同源的同名 `kind` 混淆。

建议升级：

```go
type MonitorCondition struct {
    SourceID string          `json:"source_id" toml:"source_id"`
    Kind     string          `json:"kind"      toml:"kind"`
    TargetID string          `json:"target_id,omitempty" toml:"target_id,omitempty"`
    Field    string          `json:"field"     toml:"field"`
    Op       string          `json:"op"        toml:"op"`
    Value    float64         `json:"value"     toml:"value"`
    Severity MonitorSeverity `json:"severity"  toml:"severity"`
}
```

兼容策略同 `DataRequirement`：没有 `source_id` 时按默认 builtin 源处理。

## 5. 数据类型设计

### 5.1 内置采集源

内置源继续支持现有数据类型：

| Kind | 来源 |
| --- | --- |
| `host.basic` | SSH 主机基础指标 |
| `host.disk` | SSH 磁盘水位 |
| `docker.containers` | Docker 容器状态 |
| `k8s.workloads` | K8s 工作负载状态 |
| `http.health` | HTTP 探活 |

示例：

```json
{
  "source_id": "builtin",
  "target_id": "ssh-web-01",
  "kind": "host.disk",
  "params": {
    "mount": "/"
  }
}
```

### 5.2 Prometheus 数据源

Prometheus 第一阶段支持两个 Kind：

| Kind | 说明 |
| --- | --- |
| `prometheus.query` | 即时查询，调用 `/api/v1/query` |
| `prometheus.range_query` | 区间查询，调用 `/api/v1/query_range` |

第一阶段建议优先实现 `prometheus.query`，`range_query` 可作为后续增强。

`prometheus.query` 参数：

```json
{
  "query": "up{job=\"node\"}",
  "time": "",
  "value_mode": "first"
}
```

字段说明：

| 参数 | 含义 |
| --- | --- |
| `query` | PromQL |
| `time` | 可选，Prometheus 查询时间；为空时使用当前时间 |
| `value_mode` | 多结果处理策略，第一阶段可只支持 `first` |

采集结果建议归一化为：

```go
type PrometheusQueryResult struct {
    Query  string                  `json:"query"`
    Values []PrometheusSampleValue `json:"values"`
    Value  float64                 `json:"value"`
}

type PrometheusSampleValue struct {
    Metric map[string]string `json:"metric"`
    Value  float64           `json:"value"`
}
```

其中 `Value` 是按 `value_mode` 提取出的代表值，便于普通阈值判断。

对应条件：

```json
{
  "source_id": "prom-prod",
  "kind": "prometheus.query",
  "field": "value",
  "op": ">",
  "value": 80,
  "severity": "warning"
}
```

## 6. 采集流程改造

当前流程：

```text
panels
  -> ExtractRequirements
  -> BuildCollectionPlan(target_id + kind + params)
  -> Collector
  -> DataSource by kind
  -> Batch
  -> Evaluate
```

改造后：

```text
panels
  -> ExtractRequirements
  -> BuildCollectionPlan(source_id + target_id + kind + params)
  -> 加载 MonitorSource
  -> Collector
  -> Source-aware DataSource
  -> Batch
  -> Evaluate
```

计划任务结构建议：

```go
type CollectionTask struct {
    SourceID string
    TargetID string
    Kind     string
    Params   map[string]any
    key      string
}
```

去重键：

```text
source_id | target_id | kind | canonical(params)
```

这样同一 Prometheus 查询只请求一次，同一主机磁盘采集也只执行一次。

## 7. DataSource 接口改造

当前接口：

```go
type DataSource interface {
    Kind() string
    Collect(ctx context.Context, cc CollectContext) (any, error)
}
```

多源后建议改为：

```go
type DataSource interface {
    Kind() string
    SourceKind() string
    Collect(ctx context.Context, cc CollectContext) (any, error)
}
```

或使用组合键注册：

```go
type RegistryKey struct {
    SourceKind string
    DataKind   string
}
```

推荐第二种，语义更清晰：

```go
Register("builtin", "host.disk", hostDiskSource{})
Register("prometheus", "prometheus.query", prometheusQuerySource{})
```

`CollectContext` 建议增加：

```go
type CollectContext struct {
    Env    core.EnvironmentDef
    Source core.MonitorSource
    Task   CollectionTask
    SSH    *SSHConnCache
}
```

内置源使用 `Env + TargetID` 找环境配置；Prometheus 源使用 `Source.Config` 请求 Prometheus HTTP API。

## 8. 存储设计

`MonitorStore` 增加 sources 目录：

```text
data/monitor/
  groups/
  panels/
  states/
  incidents/
  reports/
  configs/
  sources/
```

新增方法：

```go
ListSources(environmentID string) ([]core.MonitorSource, error)
GetSource(id string) (core.MonitorSource, error)
SaveSource(source core.MonitorSource) error
DeleteSource(id string) error
```

删除约束：

- 如果有监控项引用该 `source_id`，禁止删除。
- 可以允许禁用。禁用后引用该源的监控项采集失败，并在状态摘要中显示“监控源已禁用”。

内置源处理：

- 可以不落盘，按环境合成一个虚拟源。
- 也可以落盘一个 `builtin` 类型源。

推荐第一阶段使用虚拟源，减少数据迁移复杂度：

```text
source_id = "builtin"
source_kind = "builtin"
```

## 9. Wails API 设计

新增方法：

```go
ListMonitorSources(environmentID string) ([]core.MonitorSource, error)
GetMonitorSource(id string) (core.MonitorSource, error)
CreateMonitorSource(environmentID, name, kind string, config map[string]any) (string, error)
UpdateMonitorSource(source core.MonitorSource) error
DeleteMonitorSource(id string) error
TestMonitorSource(source core.MonitorSource) error
```

Prometheus 测试连接：

- 调用 `/api/v1/query?query=up`。
- 只判断 HTTP 可达、认证成功、Prometheus 返回 `status=success`。
- 测试结果不落盘。

## 10. 前端改造

新增或调整文件：

```text
frontend/src/types/monitor.ts
frontend/src/api/monitor.ts
frontend/src/features/monitor/MonitorSourcesView.tsx
frontend/src/features/monitor/CreateMonitorSourceDialog.tsx
frontend/src/features/monitor/MonitorSourceForm.tsx
frontend/src/features/monitor/PanelRequirementsEditor.tsx
frontend/src/features/monitor/PanelConditionsEditor.tsx
```

页面结构：

```text
MonitorOverview
  -> 环境选择
  -> 二级 tabs
      -> 概览
      -> 监控源
      -> 分组 / 监控项
      -> 异常历史
```

监控源表单字段：

```text
名称
类型：Prometheus
Endpoint
认证方式：无 / Basic / Bearer Token
用户名
密码
Token
超时时间
测试连接
保存
```

监控项数据需求编辑：

```text
选择监控源
选择数据类型
填写查询参数
测试本次查询
配置判断条件
```

Prometheus 需求编辑建议：

```text
PromQL 输入框
即时查询测试按钮
结果预览
阈值字段默认 value
阈值操作符和值
严重级别
```

## 11. Prometheus 客户端实现

建议新增：

```text
internal/clients/prometheus.go
internal/monitor/datasource_prometheus.go
```

客户端职责：

- 构造 HTTP 请求。
- 处理 basic / bearer 认证。
- 解析 Prometheus JSON 响应。
- 将 string 类型 sample value 转为 float64。
- 返回结构化结果。

Prometheus 响应示例：

```json
{
  "status": "success",
  "data": {
    "resultType": "vector",
    "result": [
      {
        "metric": {"instance": "web-01"},
        "value": [1710000000.0, "82.3"]
      }
    ]
  }
}
```

归一化后：

```json
{
  "query": "...",
  "value": 82.3,
  "values": [
    {
      "metric": {"instance": "web-01"},
      "value": 82.3
    }
  ]
}
```

## 12. 兼容与迁移

第一阶段需要兼容已有监控项：

- `DataRequirement.target_id` 继续有效。
- `DataRequirement.source_id` 为空时默认使用 `builtin`。
- `MonitorCondition.source_id` 为空时默认使用 `builtin`。
- 前端加载旧数据时自动补默认源，不强制立即改写 TOML。

后续可在保存监控项时写入 `source_id = "builtin"`，逐步完成自然迁移。

## 13. 安全要求

Prometheus 源可能包含内网地址和认证凭证，必须满足：

- `password`、`token` 不在列表接口明文返回。
- 日志中不打印完整认证头。
- 测试连接失败时不输出敏感 header。
- AI Prompt 默认不注入监控源凭证明文。
- 如果 Web 化部署，监控源管理接口必须鉴权。

PromQL 本身也可能暴露业务标签，例如实例名、服务名、集群名。后续若 AI 诊断使用 Prometheus 查询结果，需要按 AI 隐私策略决定是否脱敏。

## 14. 实施步骤

### Phase 1：模型与存储

- 新增 `core.MonitorSource`。
- `DataRequirement` 增加 `SourceID`。
- `MonitorCondition` 增加 `SourceID`。
- `MonitorStore` 增加 sources CRUD。
- 概览返回 sources 或新增 sources 查询接口。

验证：

- `go test ./internal/store ./internal/monitor`。
- 旧监控项数据仍能读取。

### Phase 2：采集内核改造

- `CollectionTask` 增加 `SourceID`。
- 去重键升级为 `source_id + target_id + kind + params`。
- `CollectContext` 增加 `Source`。
- `DataSource Registry` 支持 `source_kind + data_kind`。
- builtin 源兼容现有采集能力。

验证：

- 现有 host/docker/k8s/http 监控测试通过。
- 同一查询需求只采集一次。

### Phase 3：Prometheus 数据源

- 新增 Prometheus client。
- 新增 `prometheus.query` DataSource。
- 新增 `TestMonitorSource`。
- 支持 none/basic/bearer 三种认证。

验证：

- 使用 mock HTTP server 测试 Prometheus 响应解析。
- 查询失败不会影响其它采集任务。

### Phase 4：前端监控源管理

- 新增监控源列表页。
- 新增 Prometheus 表单。
- 接入测试连接。
- 监控项需求编辑支持选择监控源。

验证：

- 可以创建 Prometheus 源。
- 可以在监控项中选择 Prometheus 并配置 PromQL。
- 可以保存并重新打开。

### Phase 5：Prometheus 监控项闭环

- Prometheus 查询结果进入 Monitor Flow。
- 阈值条件可基于 `field=value` 判断。
- 异常时创建 Incident。
- 可生成诊断报告。

验证：

- 构造 Prometheus 返回值大于阈值，监控项变为异常。
- 返回值恢复正常后，状态进入正常或正常（有异常历史）。

## 15. 后续扩展

Prometheus 接入完成后，可以按同一模型扩展：

- Loki：`loki.query`、`loki.range_query`。
- Elasticsearch：`elasticsearch.query`。
- OpenSearch：`opensearch.query`。
- 云监控：`cloud.metric_query`。
- 自定义 HTTP：`custom_http.json_query`。

扩展时只需要新增：

- `MonitorSource.Kind`
- 对应 source config schema
- 对应 DataSource 实现
- 前端表单与结果预览

## 16. 关键取舍

多监控源的核心取舍是：

```text
OpsEngine 管监控项、状态、异常、诊断报告；
外部系统管原始指标和长期历史；
本轮查询结果只用于判断，正常即丢弃。
```

这样既能接入企业已有监控体系，又能保持 OpsEngine 的轻量定位，不把项目拖向完整监控平台。
