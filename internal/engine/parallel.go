// parallel 节点的并发执行
// 找所有 exec_out_1..N 端口的下游分支，每条分支 spawn 一个 goroutine
// 全部结束后才返回（让外层沿 exec_out_done 继续主流）

package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"OpsEngine/internal/core"
)

// runParallel 并发执行所有连出的 exec_out_<i> 分支
// 任一分支失败 → 整体失败，返回第一个错误
// ctx 取消时各分支内的 executeFlow 自然检测到并返回
func (r *Runtime) runParallel(
	ctx context.Context,
	parentStack *FrameStack,
	node core.NodeInstance,
	nodes []core.NodeInstance,
	edges []core.EdgeConfig,
) error {
	r.setNodeState(node.InstanceID, core.NodeStateExecuting, "")

	// 收集所有连接的分支起点
	// 端口 exec_out_done 不算分支
	var branchStarts []string
	for _, e := range edges {
		if e.From.Node != node.InstanceID {
			continue
		}
		if !strings.HasPrefix(e.From.Port, "exec_out_") || e.From.Port == "exec_out_done" {
			continue
		}
		branchStarts = append(branchStarts, e.To.Node)
	}

	if len(branchStarts) == 0 {
		r.appendLog(node.InstanceID, "info", "无并发分支连接，直接进入 done")
		return nil
	}

	r.appendLog(node.InstanceID, "info", fmt.Sprintf("启动 %d 个并发分支", len(branchStarts)))

	// spawn goroutines，等所有完成
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	for _, start := range branchStarts {
		startID := start
		// 每个分支用 fork 出的子栈，避免分支内集合调用污染兄弟分支
		branchStack := parentStack.fork()

		wg.Add(1)
		// 同时纳入 Runtime.wg，确保 main 等待
		r.wg.Add(1)
		go func() {
			defer wg.Done()
			defer r.wg.Done()
			if err := r.executeFlow(ctx, branchStack, nodes, edges, startID); err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
			}
		}()
	}

	wg.Wait()
	return firstErr
}
