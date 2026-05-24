// 数据流求值 + 执行流推进
// ctx 由调用方传入：主流用 runtime.ctx；system_over 用独立 ctx 保证可跑完

package engine

import (
	"context"

	"OpsEngine/internal/core"
)

// evalInput 求某节点的某 input 端口值
func (r *Runtime) evalInput(ctx context.Context, stack *FrameStack, nodes []core.NodeInstance, edges []core.EdgeConfig, nodeID, portID string) (any, bool) {
	for _, e := range edges {
		if e.To.Node == nodeID && e.To.Port == portID {
			return r.evalOutput(ctx, stack, nodes, edges, e.From.Node, e.From.Port)
		}
	}
	return nil, false
}

// evalOutput 求某节点的某 output 端口值
// action 节点：读 outputs 缓存
// pure 节点：按需 Execute（不缓存）
func (r *Runtime) evalOutput(ctx context.Context, stack *FrameStack, nodes []core.NodeInstance, edges []core.EdgeConfig, nodeID, portID string) (any, bool) {
	r.mu.Lock()
	cached, hasCache := r.outputs[nodeID]
	r.mu.Unlock()
	if hasCache {
		v, ok := cached[portID]
		return v, ok
	}

	node := findNode(nodes, nodeID)
	if node == nil {
		return nil, false
	}

	n, ok := Lookup(node.TypeID)
	if !ok {
		return nil, false
	}
	if n.TypeDef().NodeKind != core.NodeKindPure {
		return nil, false
	}

	c := newExecContext(ctx, r, stack, *node, nodes, edges)
	outputs, err := n.Execute(c)
	if err != nil {
		return nil, false
	}
	if v, exists := outputs[portID]; exists {
		return v, true
	}
	return nil, false
}

// executeFlow 沿 exec 流单线推进
func (r *Runtime) executeFlow(ctx context.Context, stack *FrameStack, nodes []core.NodeInstance, edges []core.EdgeConfig, startNodeID string) error {
	cur := startNodeID
	for cur != "" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		node := findNode(nodes, cur)
		if node == nil {
			return errMissingNode(cur)
		}

		// ── 引擎特判：集合调用 ───────────────────────────
		if isAssembleCallType(node.TypeID) {
			outputs, err := r.execAssembleCall(ctx, stack, *node, nodes, edges)
			if err != nil {
				r.setNodeState(cur, core.NodeStateFailed, err.Error())
				return err
			}
			r.setNodeState(cur, core.NodeStateSuccess, "")
			r.mu.Lock()
			r.outputs[cur] = outputs
			r.mu.Unlock()
			cur = r.findNextExec(edges, cur, "exec_out")
			continue
		}

		// ── 引擎特判：assemble_end ───────────────────────
		if node.TypeID == "assemble_end" {
			if err := r.runAssembleEnd(ctx, stack, *node, nodes, edges); err != nil {
				r.setNodeState(cur, core.NodeStateFailed, err.Error())
				return err
			}
			return nil
		}

		// ── 引擎特判：parallel ───────────────────────────
		if node.TypeID == "parallel" {
			if err := r.runParallel(ctx, stack, *node, nodes, edges); err != nil {
				r.setNodeState(cur, core.NodeStateFailed, err.Error())
				return err
			}
			r.setNodeState(cur, core.NodeStateSuccess, "")
			cur = r.findNextExec(edges, cur, "exec_out_done")
			continue
		}

		// ── 引擎特判：thread ─────────────────────────────
		if node.TypeID == "thread" {
			r.runThread(ctx, stack, *node, nodes, edges)
			r.setNodeState(cur, core.NodeStateSuccess, "")
			cur = r.findNextExec(edges, cur, "exec_out_continue")
			continue
		}

		// ── 引擎特判：break（终止整个工作流） ─────────────
		// 等同于用户点 Stop：cancel ctx → 跑 system_over → markTerminated
		if node.TypeID == "break" {
			r.setNodeState(cur, core.NodeStateSuccess, "")
			r.appendLog(cur, "info", "Break 触发，工作流即将终止")
			r.cancel()
			return nil
		}

		// ── 通用：从注册表拿 Node 调用 Execute ────────────
		n, ok := Lookup(node.TypeID)
		if !ok {
			r.setNodeState(cur, core.NodeStateFailed, "未注册的节点类型")
			return errUnknownType(node.TypeID)
		}

		r.setNodeState(cur, core.NodeStateExecuting, "")

		c := newExecContext(ctx, r, stack, *node, nodes, edges)
		outputs, err := n.Execute(c)
		if err != nil {
			r.setNodeState(cur, core.NodeStateFailed, err.Error())
			return err
		}
		r.setNodeState(cur, core.NodeStateSuccess, "")

		r.mu.Lock()
		r.outputs[cur] = outputs
		r.mu.Unlock()

		cur = r.findNextExec(edges, cur, "exec_out")
	}
	return nil
}

// findNextExec 沿 (fromNode, fromPort) 找下游 exec 节点的 instance_id
func (r *Runtime) findNextExec(edges []core.EdgeConfig, fromNode, fromPort string) string {
	for _, e := range edges {
		if e.From.Node == fromNode && e.From.Port == fromPort {
			return e.To.Node
		}
	}
	return ""
}
