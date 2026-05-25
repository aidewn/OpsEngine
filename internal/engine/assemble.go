// 集合调用执行：stack frame 模型（树状）
// 调用 assemble:<id> 节点时：
//   1. 在调用方 frame 求值所有 param input
//   2. 创建子 frame 挂载到 parent.Children[callerInstanceID]
//   3. 从集合内部的 assemble_start 开始执行 exec 流
//   4. assemble_end 触发时由引擎特判收集 returns 写入 frame
//   5. 把 frame.Returns 转成调用节点的 outputs

package engine

import (
	"context"
	"fmt"
	"strings"

	"OpsEngine/internal/core"
)

// execAssembleCall 执行一次集合调用
func (r *Runtime) execAssembleCall(
	ctx context.Context,
	parent *Frame,
	callNode core.NodeInstance,
	callerNodes []core.NodeInstance,
	callerEdges []core.EdgeConfig,
) (Outputs, error) {
	assembleID := strings.TrimPrefix(callNode.TypeID, "assemble:")
	asm, ok := r.Snapshot.Assembles[assembleID]
	if !ok {
		return nil, fmt.Errorf("快照中未找到集合 %s", assembleID)
	}

	r.setNodeState(parent, callNode.InstanceID, core.NodeStateExecuting, "")
	r.appendLog(parent, callNode.InstanceID, "info", fmt.Sprintf("调用集合 %s", asm.Name))

	// 1. 调用方作用域求 params
	params := map[string]any{}
	for _, p := range asm.Params {
		portID := "param_" + p.Name
		v, _ := r.evalInput(ctx, parent, callerNodes, callerEdges, callNode.InstanceID, portID)
		params[p.Name] = v
	}

	// 2. 创建子 frame
	child := r.pushChildFrame(parent, callNode.InstanceID, assembleID, initVariables(asm.Variables), params)

	// 3. 找 assemble_start 起点
	var startID string
	for _, n := range asm.Nodes {
		if n.TypeID == "assemble_start" {
			startID = n.InstanceID
			break
		}
	}
	if startID == "" {
		return nil, fmt.Errorf("集合 %s 缺少 assemble_start 节点", assembleID)
	}

	// 4. 在集合内部跑 exec 流
	if err := r.executeFlow(ctx, child, asm.Nodes, asm.Edges, startID); err != nil {
		return nil, err
	}

	// 5. 把 child.Returns 转成调用节点的 outputs
	outputs := Outputs{}
	r.mu.Lock()
	for name, value := range child.Returns {
		outputs["return_"+name] = value
	}
	r.mu.Unlock()
	return outputs, nil
}

// runAssembleEnd 处理 assemble_end 节点
func (r *Runtime) runAssembleEnd(
	ctx context.Context,
	frame *Frame,
	node core.NodeInstance,
	nodes []core.NodeInstance,
	edges []core.EdgeConfig,
) error {
	r.setNodeState(frame, node.InstanceID, core.NodeStateExecuting, "")

	if frame == nil || frame.AssembleID == "" {
		r.appendLog(frame, node.InstanceID, "warn", "assemble_end 不在集合 frame 中，已跳过")
		r.setNodeState(frame, node.InstanceID, core.NodeStateSkipped, "")
		return nil
	}
	asm, ok := r.Snapshot.Assembles[frame.AssembleID]
	if !ok {
		return fmt.Errorf("快照中未找到集合 %s", frame.AssembleID)
	}

	for _, ret := range asm.Returns {
		portID := "return_" + ret.Name
		v, _ := r.evalInput(ctx, frame, nodes, edges, node.InstanceID, portID)
		r.mu.Lock()
		frame.Returns[ret.Name] = v
		r.mu.Unlock()
	}

	r.setNodeState(frame, node.InstanceID, core.NodeStateSuccess, "")
	return nil
}

// isAssembleCallType 判断节点类型是否为集合调用
func isAssembleCallType(typeID string) bool {
	return strings.HasPrefix(typeID, "assemble:")
}

// evalAssembleStartParamOutput 求 assemble_start 的 param_<name> 输出
// 调用方传入的参数在 execAssembleCall 时已写入 child.Params
func (r *Runtime) evalAssembleStartParamOutput(frame *Frame, portID string) (any, bool) {
	if !strings.HasPrefix(portID, "param_") {
		return nil, false
	}
	name := strings.TrimPrefix(portID, "param_")
	if name == "" {
		return nil, false
	}
	return r.getParam(frame, name)
}
