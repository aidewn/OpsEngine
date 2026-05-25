// parallel 节点的并发执行
// 所有分支共享 parent frame（一个 goroutine 内调集合会创建独立子 frame）

package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"OpsEngine/internal/core"
)

// runParallel 并发执行所有连出的 exec_out_<i> 分支
func (r *Runtime) runParallel(
	ctx context.Context,
	frame *Frame,
	node core.NodeInstance,
	nodes []core.NodeInstance,
	edges []core.EdgeConfig,
) error {
	r.setNodeState(frame, node.InstanceID, core.NodeStateExecuting, "")

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
		r.appendLog(frame, node.InstanceID, "info", "无并发分支连接，直接进入 done")
		return nil
	}

	r.appendLog(frame, node.InstanceID, "info", fmt.Sprintf("启动 %d 个并发分支", len(branchStarts)))

	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	for _, start := range branchStarts {
		startID := start
		wg.Add(1)
		r.wg.Add(1)
		go func() {
			defer wg.Done()
			defer r.wg.Done()
			if err := r.executeFlow(ctx, frame, nodes, edges, startID); err != nil {
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
