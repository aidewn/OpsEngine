// KeyedGuard：按 key 的「在途」互斥标记。
// 用于保证同一环境的 tick、同一异常的诊断等，在任一时刻只有一个在执行——
// 后台调度与手动触发共用同一把锁，避免重叠采集引发报警风暴与重复诊断。

package monitor

import "sync"

// KeyedGuard 记录哪些 key 正在执行。非阻塞获取：拿不到即返回 false，由调用方决定跳过。
type KeyedGuard struct {
	mu       sync.Mutex
	inflight map[string]bool
}

// NewKeyedGuard 创建空的在途锁。
func NewKeyedGuard() *KeyedGuard {
	return &KeyedGuard{inflight: map[string]bool{}}
}

// TryAcquire 尝试占用 key：成功返回 true（调用方用完须 Release）；已被占用返回 false。
func (g *KeyedGuard) TryAcquire(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inflight[key] {
		return false
	}
	g.inflight[key] = true
	return true
}

// Release 释放 key。
func (g *KeyedGuard) Release(key string) {
	g.mu.Lock()
	delete(g.inflight, key)
	g.mu.Unlock()
}
