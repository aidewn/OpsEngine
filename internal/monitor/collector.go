// 采集执行：按计划逐任务调用 DataSource，结果按去重键汇总，可切片分发给监控项。

package monitor

import (
	"context"
	"fmt"
	"time"

	"OpsEngine/internal/core"
)

// CollectionResult 单次采集任务的结果。
// Err 非空表示该任务采集失败，Data 此时无意义。
type CollectionResult struct {
	Task        CollectionTask
	Data        any
	Err         error
	CollectedAt time.Time
}

// Batch 一轮采集的结果集合，按采集任务去重键索引。
type Batch map[string]CollectionResult

// Slice 取出某监控项关心的那部分采集结果，按其数据需求过滤。
// 监控项的 Monitor Flow 只拿到自己声明过的数据切片（plan §4.3 Evaluate）。
func (b Batch) Slice(reqs []core.DataRequirement) Batch {
	out := Batch{}
	for _, r := range reqs {
		key := collectionKey(r.TargetID, r.Kind, r.Params)
		if res, ok := b[key]; ok {
			out[key] = res
		}
	}
	return out
}

// Collector 按采集计划执行一轮批量采集。
type Collector struct {
	registry *Registry
}

// NewCollector 绑定 DataSource 注册表。
func NewCollector(registry *Registry) *Collector {
	return &Collector{registry: registry}
}

// Collect 顺序执行采集计划，逐任务调用对应 DataSource。
// 单任务失败只记在该结果的 Err 上，不影响其它任务——一个目标不可达不该拖垮整轮。
// 正常数据不持久化（plan §5.1）：结果只在内存返回给调用方判断。
func (c *Collector) Collect(ctx context.Context, env core.EnvironmentDef, plan []CollectionTask) Batch {
	// 本轮共享一个 SSH 连接缓存：同一目标只拨号一次，tick 结束统一关闭。
	sshCache := NewSSHConnCache(env)
	defer sshCache.Close()

	batch := Batch{}
	for _, task := range plan {
		res := CollectionResult{Task: task, CollectedAt: time.Now()}
		if ds, ok := c.registry.Get(task.Kind); ok {
			res.Data, res.Err = ds.Collect(ctx, CollectContext{Env: env, Task: task, SSH: sshCache})
		} else {
			res.Err = fmt.Errorf("未注册的数据类型: %s", task.Kind)
		}
		batch[task.Key()] = res
	}
	return batch
}

// RunCollection 执行一轮完整采集：汇总需求 → 去重计划 → 批量采集。
// 对应 plan §4.3 的 Plan + Collect 两步；Evaluate（分发判断）留给 Phase 3。
func (c *Collector) RunCollection(ctx context.Context, env core.EnvironmentDef, panels []core.MonitorPanel) Batch {
	plan := BuildCollectionPlan(ExtractRequirements(panels))
	return c.Collect(ctx, env, plan)
}
