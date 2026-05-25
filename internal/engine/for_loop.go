// for_loop 节点的迭代执行
//
// 关键点：
//   - 半开区间 [start, end)；start >= end 直接走 done 不进 body
//   - for_loop 的 NodeKind=FlowControl，不会被 evalOutput 当作 pure 求值，
//     因此 index 必须在每次迭代前由 runFor 显式写入 frame.Outputs[forID]，
//     下游 var_get / math_add 才能通过 evalOutput 的缓存路径读到当前值
//   - body 流末端没有下一节点会自然 return（executeFlow 的 cur=="" 出口），
//     回到 runFor 的循环里继续下一轮，无需特殊"循环回边"
//   - 取消：每次迭代开头检查 ctx.Done()；body 流内部也会沿用同一 ctx
//   - body 内的 action 输出会在 frame.Outputs 上覆盖（每轮迭代写同一 instanceID），
//     这是 OK 的——下一轮会重新执行；前端只能看到最新一次的输出，调试时知悉即可

package engine

import (
	"context"

	"OpsEngine/internal/core"
)

// runFor 执行计数循环
func (r *Runtime) runFor(
	ctx context.Context,
	frame *Frame,
	node core.NodeInstance,
	nodes []core.NodeInstance,
	edges []core.EdgeConfig,
) error {
	r.setNodeState(frame, node.InstanceID, core.NodeStateExecuting, "")

	startV, _ := r.evalInput(ctx, frame, nodes, edges, node.InstanceID, "start")
	endV, _ := r.evalInput(ctx, frame, nodes, edges, node.InstanceID, "end")
	start := coerceInt64(startV)
	end := coerceInt64(endV)

	bodyStart := r.findNextExec(edges, node.InstanceID, "exec_out_body")
	if bodyStart == "" {
		r.appendLog(frame, node.InstanceID, "warn", "exec_out_body 未连接，跳过循环体")
		return nil
	}

	if start >= end {
		r.appendLog(frame, node.InstanceID, "info", "区间为空 [start>=end]，不执行循环体")
		return nil
	}

	for i := start; i < end; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 写入 index 输出，让 body 内的下游通过 evalOutput 缓存读取
		r.mu.Lock()
		o := frame.Outputs[node.InstanceID]
		if o == nil {
			o = Outputs{}
			frame.Outputs[node.InstanceID] = o
		}
		o["index"] = i
		r.mu.Unlock()

		if err := r.executeFlow(ctx, frame, nodes, edges, bodyStart); err != nil {
			return err
		}
	}
	return nil
}

// coerceInt64 容错把 any 转 int64，行为对齐 internal/nodes/exprhelper.ToInt64
// 与 coerceBool 同样，保留在 engine 包内避免反向依赖 nodes 子包
func coerceInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case float64:
		return int64(x)
	case float32:
		return int64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	}
	return 0
}
