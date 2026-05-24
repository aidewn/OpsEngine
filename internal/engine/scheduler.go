// system_update 周期触发调度器
// 仅支持 interval（time.Ticker）和 manual（不主动触发）
// cron 留到后续迭代

package engine

import (
	"context"
	"strings"
	"sync"
	"time"

	"OpsEngine/internal/core"
)

// Scheduler 单个 Runtime 的调度器
type Scheduler struct {
	runtime *Runtime
	tickers []*time.Ticker
	// 重叠抑制：上次 update 流还在跑时，本次 tick 跳过
	updateMu      sync.Mutex
	updateRunning bool
}

func newScheduler(rt *Runtime) *Scheduler {
	return &Scheduler{runtime: rt}
}

// Start 找到 system_update 节点并按 config 启动周期触发
// 返回 true 表示存在 system_update 节点（runtime 需要等用户停止才退出）
func (s *Scheduler) Start(ctx context.Context, nodes []core.NodeInstance, edges []core.EdgeConfig) bool {
	var updateNode *core.NodeInstance
	for i := range nodes {
		if nodes[i].TypeID == "system_update" {
			updateNode = &nodes[i]
			break
		}
	}
	if updateNode == nil {
		return false
	}

	deltaType, _ := updateNode.Config["delta_type"].(string)
	if deltaType == "" {
		deltaType = "interval"
	}

	switch deltaType {
	case "interval":
		seconds := readSeconds(updateNode.Config["delta_seconds"])
		if seconds <= 0 {
			seconds = 60
		}
		s.startInterval(ctx, *updateNode, nodes, edges, time.Duration(seconds)*time.Second)
		return true

	case "cron":
		// Cron 留到后续迭代
		s.runtime.appendLog(updateNode.InstanceID, "warn", "cron 触发方式暂未支持，工作流将不会周期触发")
		return true // 仍认为有 update：runtime 不会立即退出，可由用户控制停止

	case "manual":
		// 不主动触发；runtime 仍存活等待用户操作
		return true

	default:
		s.runtime.appendLog(updateNode.InstanceID, "warn", "未知的 delta_type: "+deltaType)
		return false
	}
}

// startInterval 用 time.Ticker 周期触发 update 流
func (s *Scheduler) startInterval(
	ctx context.Context,
	updateNode core.NodeInstance,
	nodes []core.NodeInstance,
	edges []core.EdgeConfig,
	interval time.Duration,
) {
	ticker := time.NewTicker(interval)
	s.tickers = append(s.tickers, ticker)

	s.runtime.wg.Add(1)
	go func() {
		defer s.runtime.wg.Done()
		s.runtime.appendLog(updateNode.InstanceID, "info", "启动周期触发 ("+interval.String()+")")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.triggerUpdate(ctx, updateNode, nodes, edges)
			}
		}
	}()
}

// triggerUpdate 触发一次 update 流（从 system_update.exec_out 起点开始）
// 抑制重叠：上次还没跑完则跳过本次 tick
func (s *Scheduler) triggerUpdate(
	ctx context.Context,
	updateNode core.NodeInstance,
	nodes []core.NodeInstance,
	edges []core.EdgeConfig,
) {
	s.updateMu.Lock()
	if s.updateRunning {
		s.updateMu.Unlock()
		s.runtime.appendLog(updateNode.InstanceID, "warn", "上次 update 流尚未结束，本次跳过")
		return
	}
	s.updateRunning = true
	s.updateMu.Unlock()
	defer func() {
		s.updateMu.Lock()
		s.updateRunning = false
		s.updateMu.Unlock()
	}()

	next := s.runtime.findNextExec(edges, updateNode.InstanceID, "exec_out")
	if next == "" {
		return
	}
	// update 流在主栈上跑（共享主帧变量）
	stack := s.runtime.newMainStack()
	if err := s.runtime.executeFlow(ctx, stack, nodes, edges, next); err != nil {
		s.runtime.appendLog(updateNode.InstanceID, "error", "update 流失败: "+err.Error())
	}
}

// Stop 停止所有定时器（goroutine 会通过 ctx.Done() 自然退出）
func (s *Scheduler) Stop() {
	for _, t := range s.tickers {
		t.Stop()
	}
}

// readSeconds 兼容 int64 / int / float64 / string 几种 config 序列化形态
func readSeconds(raw any) int64 {
	switch v := raw.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		// 去掉单位（如 "60s"）的容错
		v = strings.TrimSpace(v)
		if v == "" {
			return 0
		}
		// 简单 atoi（避免引 strconv）
		var n int64
		for _, ch := range v {
			if ch < '0' || ch > '9' {
				break
			}
			n = n*10 + int64(ch-'0')
		}
		return n
	}
	return 0
}
