// branch 节点的执行：求 condition 选 exec_out_true 或 exec_out_false
//
// 设计要点：
//   - 不在 internal/nodes/branch_node 里写逻辑，与 parallel/thread/break 一致：
//     节点包仅声明元数据，引擎特判推进
//   - condition 未连接 / 类型不可识别时按 false 处理（容错）
//   - 不引用 internal/nodes/exprhelper 以避免 engine 反向依赖 nodes
//     coerceBool 在 engine 包内独立维护，与 exprhelper.ToBool 行为对齐

package engine

import (
	"context"

	"OpsEngine/internal/core"
)

// runBranch 求值 condition input，返回应推进的 exec 端口 ID
// 调用方（evaluator.executeFlow）拿到端口 ID 后用 findNextExec 查下一节点
func (r *Runtime) runBranch(
	ctx context.Context,
	frame *Frame,
	node core.NodeInstance,
	nodes []core.NodeInstance,
	edges []core.EdgeConfig,
) (string, error) {
	r.setNodeState(frame, node.InstanceID, core.NodeStateExecuting, "")

	cond, _ := r.evalInput(ctx, frame, nodes, edges, node.InstanceID, "condition")
	if coerceBool(cond) {
		r.appendLog(frame, node.InstanceID, "info", "条件为 true，走 true 分支")
		return "exec_out_true", nil
	}
	r.appendLog(frame, node.InstanceID, "info", "条件为 false，走 false 分支")
	return "exec_out_false", nil
}

// coerceBool 容错把 any 转 bool，用于流控节点（branch / 未来的 while_loop）的条件求值
// 行为与 internal/nodes/exprhelper.ToBool 对齐；保留在 engine 包内避免 engine 反向依赖 nodes 子包
//
// 规则：
//   - bool: 原样
//   - 数字非零: true
//   - 字符串 "true" / "1": true，其余: false
//   - 其他类型 / nil: false
func coerceBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int64:
		return x != 0
	case int:
		return x != 0
	case int32:
		return x != 0
	case float64:
		return x != 0
	case float32:
		return x != 0
	case string:
		return x == "true" || x == "1"
	}
	return false
}
