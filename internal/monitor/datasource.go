// 监控采集管线（Phase 2：轻量 DataSource 与批量采集）。
//
// 设计取舍（对齐 docs/monitoring-architecture-plan.md §4）：
//   - DataSource 只负责「描述可采集数据 + 执行一轮采集」，不长期保存数据（§5.1）；
//   - 采集按 监控源+目标+类型+参数 去重后批量执行，避免监控项数量增长打爆服务器（§4.1）；
//   - 本包是采集内核：planner 汇总去重、collector 执行分发，均为纯逻辑、可单测。
//     真实采集器（host/docker/k8s/http）作为 DataSource 实现后续接入，不在内核内硬编码。

package monitor

import (
	"context"
	"fmt"

	"OpsEngine/internal/core"
)

// CollectContext 传给 DataSource 的采集上下文：环境（含凭证引用）+ 本次采集任务。
// SSH 为本轮 tick 共享的连接缓存（可为 nil，此时数据源各自拨号），用于同目标连接复用。
type CollectContext struct {
	Env    core.EnvironmentDef
	Source core.MonitorSource
	Task   CollectionTask
	SSH    *SSHConnCache
}

// DataSource 描述某一类数据（Kind）的采集能力。
// 实现需保持无状态：同一实例会被复用于同一类型、不同目标的多次采集。
type DataSource interface {
	// SourceKind 返回该采集器所属监控源类型（如 builtin、prometheus）。
	SourceKind() string
	// Kind 返回该数据源处理的数据类型（如 host.basic、http.health）。
	Kind() string
	// Collect 执行一次采集，返回结构化数据或错误。
	Collect(ctx context.Context, cc CollectContext) (any, error)
}

// Registry 按 source kind + data kind 索引 DataSource。
type Registry struct {
	sources map[string]DataSource
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{sources: map[string]DataSource{}}
}

// Register 注册一个 DataSource；SourceKind/Kind 为空或重复均视为错误。
func (r *Registry) Register(ds DataSource) error {
	sourceKind := ds.SourceKind()
	kind := ds.Kind()
	if sourceKind == "" {
		return fmt.Errorf("DataSource SourceKind 不能为空")
	}
	if kind == "" {
		return fmt.Errorf("DataSource Kind 不能为空")
	}
	key := registryKey(sourceKind, kind)
	if _, exists := r.sources[key]; exists {
		return fmt.Errorf("DataSource 重复注册: %s/%s", sourceKind, kind)
	}
	r.sources[key] = ds
	return nil
}

// Get 按 source kind + data kind 取 DataSource。
func (r *Registry) Get(sourceKind, kind string) (DataSource, bool) {
	ds, ok := r.sources[registryKey(sourceKind, kind)]
	return ds, ok
}

// registryKey 生成 DataSource 注册键。
func registryKey(sourceKind, kind string) string {
	return sourceKind + "|" + kind
}
