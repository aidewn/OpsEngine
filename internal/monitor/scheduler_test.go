// 调度器单测：首轮触发、长间隔不重复触发、无启用环境不触发。

package monitor

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduler_RunsFirstTickAndRespectsInterval(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	fired := make(chan string, 10)
	sched := NewScheduler(
		func() []EnvSchedule { return []EnvSchedule{{EnvironmentID: "env-1", IntervalSeconds: 3600}} },
		func(id string) {
			mu.Lock()
			calls[id]++
			mu.Unlock()
			fired <- id
		},
		10*time.Millisecond, 2, nil,
	)
	sched.Start()
	defer sched.Stop()

	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("首轮 tick 未在预期时间内触发")
	}
	// 间隔 1 小时，再等若干基准节拍不应再次触发。
	time.Sleep(60 * time.Millisecond)
	mu.Lock()
	n := calls["env-1"]
	mu.Unlock()
	if n != 1 {
		t.Fatalf("长间隔环境应只触发一次，实际 %d", n)
	}
}

func TestScheduler_SkipsWhenNoSchedules(t *testing.T) {
	fired := make(chan struct{}, 1)
	sched := NewScheduler(
		func() []EnvSchedule { return nil },
		func(string) { fired <- struct{}{} },
		10*time.Millisecond, 2, nil,
	)
	sched.Start()
	defer sched.Stop()

	select {
	case <-fired:
		t.Fatal("无启用环境不应触发 tick")
	case <-time.After(80 * time.Millisecond):
	}
}

// 全局并发上限：多个环境同时到期时，同时执行的 tick 数不超过 maxConcurrent。
func TestScheduler_RespectsMaxConcurrent(t *testing.T) {
	const maxConcurrent = 2
	var current, peak int32
	release := make(chan struct{})

	schedules := []EnvSchedule{
		{EnvironmentID: "e1", IntervalSeconds: 3600},
		{EnvironmentID: "e2", IntervalSeconds: 3600},
		{EnvironmentID: "e3", IntervalSeconds: 3600},
		{EnvironmentID: "e4", IntervalSeconds: 3600},
	}
	sched := NewScheduler(
		func() []EnvSchedule { return schedules },
		func(string) {
			n := atomic.AddInt32(&current, 1)
			for {
				p := atomic.LoadInt32(&peak)
				if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
					break
				}
			}
			<-release // 阻塞，制造并发占用
			atomic.AddInt32(&current, -1)
		},
		10*time.Millisecond, maxConcurrent, nil,
	)
	sched.Start()
	defer sched.Stop()

	// 等到达到上限（应稳定在 maxConcurrent，多余的在排队）。
	deadline := time.After(time.Second)
	for atomic.LoadInt32(&current) < maxConcurrent {
		select {
		case <-deadline:
			t.Fatalf("未在预期时间内达到并发上限，current=%d", atomic.LoadInt32(&current))
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	// 再观察一小段，峰值不得超过上限。
	time.Sleep(50 * time.Millisecond)
	if p := atomic.LoadInt32(&peak); p > maxConcurrent {
		t.Fatalf("并发峰值 %d 超过上限 %d", p, maxConcurrent)
	}
	close(release)
}
