// while_loop 节点的迭代执行
//
// 关键点：
//   - 每次迭代前重新调 evalInput("condition")。pure 链路（compare_* + var_get）
//     会被 evalOutput 重新触发 Execute，因此能反映 body 里更新过的变量值。
//     若用户把 action 节点输出连到 condition，那个输出会在 frame.Outputs 上被缓存，
//     导致 while 死循环——这是用户编排错误，由日志和文档提醒，引擎不强制检查。
//   - condition 未连或类型不可识别按 false 处理（与 branch 一致）→ 直接走 done
//   - 取消：每次迭代开头 select ctx.Done()；body 共享同一 ctx
//   - body 末端没有下一节点会自然 return，回到 runWhile 主循环

package engine

import (
	"context"

	"OpsEngine/internal/core"
)

// runWhile 执行条件循环
func (r *Runtime) runWhile(
	ctx context.Context,
	frame *Frame,
	node core.NodeInstance,
	nodes []core.NodeInstance,
	edges []core.EdgeConfig,
) error {
	r.setNodeState(frame, node.InstanceID, core.NodeStateExecuting, "")

	bodyStart := r.findNextExec(edges, node.InstanceID, "exec_out_body")
	if bodyStart == "" {
		r.appendLog(frame, node.InstanceID, "warn", "exec_out_body 未连接，跳过循环")
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		cond, _ := r.evalInput(ctx, frame, nodes, edges, node.InstanceID, "condition")
		if !coerceBool(cond) {
			return nil
		}

		if err := r.executeFlow(ctx, frame, nodes, edges, bodyStart); err != nil {
			return err
		}
	}
}
