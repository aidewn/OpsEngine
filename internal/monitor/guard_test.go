// KeyedGuard 单测：同 key 互斥、释放后可再获取、不同 key 互不影响。

package monitor

import "testing"

func TestKeyedGuard(t *testing.T) {
	g := NewKeyedGuard()

	if !g.TryAcquire("env-1") {
		t.Fatal("首次获取应成功")
	}
	if g.TryAcquire("env-1") {
		t.Fatal("同 key 在途时再次获取应失败")
	}
	if !g.TryAcquire("env-2") {
		t.Fatal("不同 key 应可独立获取")
	}

	g.Release("env-1")
	if !g.TryAcquire("env-1") {
		t.Fatal("释放后应可再次获取")
	}
}
