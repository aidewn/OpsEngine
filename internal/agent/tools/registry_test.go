// Registry 单测：注册 / 查找 / 拒绝高权限 / 拒绝重名。

package tools

import (
	"strings"
	"testing"
)

// fakeTool 是测试用的最小工具，只携带 Spec。
type fakeTool struct{ spec Spec }

func (f fakeTool) Spec() Spec                                       { return f.spec }
func (f fakeTool) Execute(ToolContext, map[string]any) (Result, error) { return Result{}, nil }

func TestRegistryRegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(fakeTool{spec: Spec{Name: "a", Tier: TierRead}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Lookup("a"); !ok {
		t.Fatal("Lookup 失败")
	}
	if _, ok := r.Lookup("missing"); ok {
		t.Fatal("不存在的工具不应命中")
	}
}

func TestRegistryRejectsHighTier(t *testing.T) {
	r := NewRegistry()
	err := r.Register(fakeTool{spec: Spec{Name: "rm", Tier: TierHighWrite}})
	if err == nil || !strings.Contains(err.Error(), "未启用") {
		t.Fatalf("expected high-tier reject, got %v", err)
	}
}

func TestRegistryRejectsDuplicate(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(fakeTool{spec: Spec{Name: "a", Tier: TierRead}})
	if err := r.Register(fakeTool{spec: Spec{Name: "a", Tier: TierRead}}); err == nil {
		t.Fatal("expected duplicate reject")
	}
}

func TestRegistryListSorted(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(fakeTool{spec: Spec{Name: "b", Tier: TierRead}})
	_ = r.Register(fakeTool{spec: Spec{Name: "a", Tier: TierRead}})
	list := r.List()
	if len(list) != 2 || list[0].Spec().Name != "a" || list[1].Spec().Name != "b" {
		t.Fatalf("排序异常: %v", list)
	}
}

func TestTruncateOutput(t *testing.T) {
	in := strings.Repeat("x", MaxOutputBytes+100)
	out := TruncateOutput(in)
	if !strings.HasSuffix(out, "(输出已截断)") {
		t.Fatal("超长内容未截断")
	}
}
