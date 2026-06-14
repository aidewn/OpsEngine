// 采集管线内核单测：覆盖需求汇总、去重计划、批量采集的分发与错误隔离。

package monitor

import (
	"context"
	"errors"
	"testing"

	"OpsEngine/internal/core"
)

// fakeDataSource 记录每个去重键被采集的次数，用于验证「合并采集只跑一次」。
type fakeDataSource struct {
	kind  string
	calls map[string]int
	err   error // 非空时所有采集返回该错误
}

func newFakeDataSource(kind string) *fakeDataSource {
	return &fakeDataSource{kind: kind, calls: map[string]int{}}
}

func (f *fakeDataSource) Kind() string { return f.kind }

func (f *fakeDataSource) Collect(_ context.Context, cc CollectContext) (any, error) {
	f.calls[cc.Task.Key()]++
	if f.err != nil {
		return nil, f.err
	}
	return "data:" + cc.Task.TargetID, nil
}

// panel 构造一个启用的监控项样本。
func panel(id string, enabled bool, reqs ...core.DataRequirement) core.MonitorPanel {
	return core.MonitorPanel{ID: id, Enabled: enabled, Requirements: reqs}
}

func req(target, kind string, params map[string]any) core.DataRequirement {
	return core.DataRequirement{TargetID: target, Kind: kind, Params: params}
}

// ExtractRequirements 跳过未启用监控项，保留启用监控项的全部需求（不去重）。
func TestExtractRequirements_SkipsDisabled(t *testing.T) {
	panels := []core.MonitorPanel{
		panel("p1", true, req("web-01", "host.basic", nil)),
		panel("p2", false, req("web-01", "host.disk", nil)), // 未启用，应跳过
		panel("p3", true, req("web-01", "host.basic", nil)), // 与 p1 重复，此阶段不去重
	}
	reqs := ExtractRequirements(panels)
	if len(reqs) != 2 {
		t.Fatalf("期望 2 条需求（跳过未启用、暂不去重），实际 %d", len(reqs))
	}
}

// BuildCollectionPlan 按 目标+类型+参数 去重。
func TestBuildCollectionPlan_Dedup(t *testing.T) {
	reqs := []core.DataRequirement{
		req("web-01", "host.disk", map[string]any{"mount": "/"}),
		req("web-01", "host.disk", map[string]any{"mount": "/"}),     // 完全相同，去重
		req("web-01", "host.disk", map[string]any{"mount": "/data"}), // 参数不同，保留
		req("web-02", "host.disk", map[string]any{"mount": "/"}),     // 目标不同，保留
		req("web-01", "host.basic", nil),                             // 类型不同，保留
	}
	plan := BuildCollectionPlan(reqs)
	if len(plan) != 4 {
		t.Fatalf("期望去重后 4 个任务，实际 %d: %+v", len(plan), plan)
	}
}

// nil 参数与空 map 视为同一采集键，避免重复采集。
func TestCollectionKey_NilAndEmptyParamsEqual(t *testing.T) {
	if collectionKey("t", "k", nil) != collectionKey("t", "k", map[string]any{}) {
		t.Fatal("nil 参数与空 map 应产生相同去重键")
	}
}

// 两个监控项引用同一采集：DataSource 只被调用一次，且各自能切片拿到结果。
func TestCollector_DedupAndDistribute(t *testing.T) {
	ds := newFakeDataSource("host.basic")
	reg := NewRegistry()
	if err := reg.Register(ds); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	collector := NewCollector(reg)

	shared := req("web-01", "host.basic", nil)
	p1 := panel("p1", true, shared)
	p2 := panel("p2", true, shared)

	batch := collector.RunCollection(context.Background(), core.EnvironmentDef{}, []core.MonitorPanel{p1, p2})

	// 合并采集：同一目标同一数据只采一次
	if got := ds.calls[collectionKey("web-01", "host.basic", nil)]; got != 1 {
		t.Fatalf("期望合并后只采集 1 次，实际 %d", got)
	}
	// 分发：每个监控项都能切片拿到自己的结果
	for _, p := range []core.MonitorPanel{p1, p2} {
		slice := batch.Slice(p.Requirements)
		if len(slice) != 1 {
			t.Fatalf("监控项 %s 切片应含 1 条结果，实际 %d", p.ID, len(slice))
		}
		for _, res := range slice {
			if res.Err != nil || res.Data != "data:web-01" {
				t.Fatalf("监控项 %s 结果异常: data=%v err=%v", p.ID, res.Data, res.Err)
			}
		}
	}
}

// 未注册的数据类型：对应任务结果带错误，不 panic。
func TestCollector_UnknownKind(t *testing.T) {
	collector := NewCollector(NewRegistry())
	p := panel("p1", true, req("web-01", "mystery.kind", nil))

	batch := collector.RunCollection(context.Background(), core.EnvironmentDef{}, []core.MonitorPanel{p})
	res, ok := batch[collectionKey("web-01", "mystery.kind", nil)]
	if !ok {
		t.Fatal("未注册类型也应在 batch 中留下结果条目")
	}
	if res.Err == nil {
		t.Fatal("未注册类型应返回错误")
	}
}

// 单任务采集失败不影响其它任务（错误隔离）。
func TestCollector_PerTaskErrorIsolation(t *testing.T) {
	okDS := newFakeDataSource("host.basic")
	badDS := newFakeDataSource("host.disk")
	badDS.err = errors.New("目标不可达")

	reg := NewRegistry()
	_ = reg.Register(okDS)
	_ = reg.Register(badDS)
	collector := NewCollector(reg)

	p := panel("p1", true,
		req("web-01", "host.basic", nil),
		req("web-01", "host.disk", nil),
	)
	batch := collector.RunCollection(context.Background(), core.EnvironmentDef{}, []core.MonitorPanel{p})

	good := batch[collectionKey("web-01", "host.basic", nil)]
	bad := batch[collectionKey("web-01", "host.disk", nil)]
	if good.Err != nil {
		t.Fatalf("正常任务不应有错误: %v", good.Err)
	}
	if bad.Err == nil {
		t.Fatal("失败任务应保留错误")
	}
}

// 重复注册同一 Kind 应报错。
func TestRegistry_DuplicateRegister(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(newFakeDataSource("host.basic")); err != nil {
		t.Fatalf("首次注册失败: %v", err)
	}
	if err := reg.Register(newFakeDataSource("host.basic")); err == nil {
		t.Fatal("重复注册应报错")
	}
}

// Slice 只返回监控项声明过的采集键，忽略无关结果。
func TestBatch_SliceFiltersUnrelated(t *testing.T) {
	ds := newFakeDataSource("host.basic")
	reg := NewRegistry()
	_ = reg.Register(ds)
	collector := NewCollector(reg)

	p1 := panel("p1", true, req("web-01", "host.basic", nil))
	p2 := panel("p2", true, req("web-02", "host.basic", nil))
	batch := collector.RunCollection(context.Background(), core.EnvironmentDef{}, []core.MonitorPanel{p1, p2})

	slice := batch.Slice(p1.Requirements)
	if len(slice) != 1 {
		t.Fatalf("p1 切片应只含自己的 1 条结果，实际 %d", len(slice))
	}
	if _, ok := slice[collectionKey("web-02", "host.basic", nil)]; ok {
		t.Fatal("切片不应包含其它监控项的结果")
	}
}
