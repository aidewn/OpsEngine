// thread 节点的后台 spawn
// 主流立即沿 exec_out_continue 推进
// exec_out_thread 链在独立 goroutine 中执行，生命周期受 Runtime.ctx 管理

package engine

import (
	"context"
	"errors"
	"fmt"

	"OpsEngine/internal/core"
)

// runThread spawn 一个后台 goroutine 跑 exec_out_thread 分支
// 主流不阻塞；后台线程被纳入 Runtime.wg，runMain 会等其结束
// 后台线程沿用主流的 ctx：主流被取消时它也立即退出
func (r *Runtime) runThread(
	ctx context.Context,
	parentStack *FrameStack,
	node core.NodeInstance,
	nodes []core.NodeInstance,
	edges []core.EdgeConfig,
) {
	r.setNodeState(node.InstanceID, core.NodeStateExecuting, "")

	// 找 thread 分支起点
	var threadStart string
	for _, e := range edges {
		if e.From.Node == node.InstanceID && e.From.Port == "exec_out_thread" {
			threadStart = e.To.Node
			break
		}
	}

	if threadStart == "" {
		r.appendLog(node.InstanceID, "warn", "exec_out_thread 未连接，跳过 spawn")
		return
	}

	r.appendLog(node.InstanceID, "info", "启动后台线程")

	// fork 一份栈给独立线程
	threadStack := parentStack.fork()
	startID := threadStart

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		if err := r.executeFlow(ctx, threadStack, nodes, edges, startID); err != nil {
			if errors.Is(err, ctx.Err()) {
				return
			}
			r.appendLog(node.InstanceID, "error", fmt.Sprintf("后台线程失败: %v", err))
		}
	}()
}
