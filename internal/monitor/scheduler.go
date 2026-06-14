// 后台监控调度器（plan §4.3）：按各环境配置的间隔，周期性自动驱动一轮 tick。
//
// 设计：单个轮询循环 + lastRun 记录，每个基准节拍检查哪些环境到期。
//   - 依赖以回调注入（列出启用环境、执行一轮 tick），与 store/App 解耦、便于单测；
//   - 运行时读取最新调度表，用户改配置（开关/间隔）下一节拍即生效，无需重启循环；
//   - 同一环境的 tick 不重叠：上一轮未结束则跳过本轮，避免慢采集堆积。

package monitor

import (
	"sync"
	"time"
)

// EnvSchedule 描述一个启用监控的环境及其采集间隔。
type EnvSchedule struct {
	EnvironmentID   string
	IntervalSeconds int
}

// Scheduler 周期调度器。
type Scheduler struct {
	listSchedules func() []EnvSchedule       // 返回当前启用监控的环境及间隔
	runTick       func(environmentID string) // 执行一轮 tick（阻塞，内部自带超时）
	baseInterval  time.Duration              // 调度器基准节拍（到期判定粒度）
	sem           chan struct{}              // 全局并发上限：限制同时执行的环境 tick 数
	logf          func(format string, args ...any)

	stop    chan struct{}
	stopped chan struct{}

	mu      sync.Mutex
	lastRun map[string]time.Time
	running map[string]bool
}

// NewScheduler 构造调度器。
//   - baseInterval<=0 取默认 10s；
//   - maxConcurrent<=0 取默认 2：限制同时执行的环境 tick 数，避免环境多时资源尖峰；
//   - logf 为 nil 时丢弃日志。
func NewScheduler(
	listSchedules func() []EnvSchedule,
	runTick func(environmentID string),
	baseInterval time.Duration,
	maxConcurrent int,
	logf func(format string, args ...any),
) *Scheduler {
	if baseInterval <= 0 {
		baseInterval = 10 * time.Second
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 2
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Scheduler{
		listSchedules: listSchedules,
		runTick:       runTick,
		baseInterval:  baseInterval,
		sem:           make(chan struct{}, maxConcurrent),
		logf:          logf,
		lastRun:       map[string]time.Time{},
		running:       map[string]bool{},
	}
}

// Start 启动后台循环（非阻塞）。重复调用前需先 Stop。
func (s *Scheduler) Start() {
	s.stop = make(chan struct{})
	s.stopped = make(chan struct{})
	go s.loop()
}

// Stop 停止后台循环并等待退出；未启动时为 no-op。
func (s *Scheduler) Stop() {
	if s.stop == nil {
		return
	}
	close(s.stop)
	<-s.stopped
	s.stop = nil
}

func (s *Scheduler) loop() {
	defer close(s.stopped)
	ticker := time.NewTicker(s.baseInterval)
	defer ticker.Stop()
	s.dispatchDue() // 启动即评估一次：lastRun 为空的启用环境会立刻跑首轮
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.dispatchDue()
		}
	}
}

// dispatchDue 检查各启用环境是否到期，到期且未在运行则派发一次 tick。
func (s *Scheduler) dispatchDue() {
	now := time.Now()
	for _, sch := range s.listSchedules() {
		interval := time.Duration(sch.IntervalSeconds) * time.Second
		if interval <= 0 {
			continue
		}
		s.mu.Lock()
		last, has := s.lastRun[sch.EnvironmentID]
		due := !has || now.Sub(last) >= interval
		if due && !s.running[sch.EnvironmentID] {
			s.running[sch.EnvironmentID] = true
			s.lastRun[sch.EnvironmentID] = now
			s.mu.Unlock()
			go s.runOne(sch.EnvironmentID)
		} else {
			s.mu.Unlock()
		}
	}
}

// runOne 执行单环境 tick，受全局并发上限约束；结束后清除运行标记；panic 不致命。
// 阻塞在信号量期间 running 标记仍保持，故该环境不会被重复派发（不会堆积）。
func (s *Scheduler) runOne(environmentID string) {
	s.sem <- struct{}{} // 占用一个并发额度（满则排队）
	defer func() {
		<-s.sem
		s.mu.Lock()
		s.running[environmentID] = false
		s.mu.Unlock()
		if r := recover(); r != nil {
			s.logf("监控 tick 异常 env=%s: %v", environmentID, r)
		}
	}()
	s.runTick(environmentID)
}
