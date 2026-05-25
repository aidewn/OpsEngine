// thread 节点的后台 spawn
// 后台线程沿用主流的 ctx：主流被取消时它也立即退出

package engine

import (
	"context"
	"errors"
	"fmt"

	"OpsEngine/internal/core"
)

// runThread spawn 一个后台 goroutine 跑 exec_out_thread 分支
func (r *Runtime) runThread(
	ctx context.Context,
	frame *Frame,
	node core.NodeInstance,
	nodes []core.NodeInstance,
	edges []core.EdgeConfig,
) {
	r.setNodeState(frame, node.InstanceID, core.NodeStateExecuting, "")

	var threadStart string
	for _, e := range edges {
		if e.From.Node == node.InstanceID && e.From.Port == "exec_out_thread" {
			threadStart = e.To.Node
			break
		}
	}

	if threadStart == "" {
		r.appendLog(frame, node.InstanceID, "warn", "exec_out_thread 未连接，跳过 spawn")
		return
	}

	r.appendLog(frame, node.InstanceID, "info", "启动后台线程")

	startID := threadStart

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		if err := r.executeFlow(ctx, frame, nodes, edges, startID); err != nil {
			if errors.Is(err, ctx.Err()) {
				return
			}
			r.appendLog(frame, node.InstanceID, "error", fmt.Sprintf("后台线程失败: %v", err))
		}
	}()
}
