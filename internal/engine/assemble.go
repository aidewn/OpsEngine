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
	"strconv"
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
	// 端口未连线时回退到 caller 节点 config 里的 param_<name>_default
	// 默认值约定：FieldSchema 用 number/toggle/text，最终值已是数字/bool/string；
	// 若历史/手工配置存成字符串，按 VarType 解析一次，失败则用 nil（不阻断执行）
	params := map[string]any{}
	for _, p := range asm.Params {
		portID := "param_" + p.Name
		if v, ok := r.evalInput(ctx, parent, callerNodes, callerEdges, callNode.InstanceID, portID); ok {
			params[p.Name] = v
			continue
		}
		params[p.Name] = resolveAssembleParamDefault(callNode.Config[portID+"_default"], p.VarType)
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

// resolveAssembleParamDefault 把 caller 节点 config 中的默认值按 VarType 规范化
// raw 可能是 number/bool/string（取决于前端 FieldSchema 类型）；空值/解析失败统一返回 nil
func resolveAssembleParamDefault(raw any, varType core.PortType) any {
	if raw == nil {
		return nil
	}
	switch varType {
	case core.PortTypeInt:
		switch v := raw.(type) {
		case float64:
			return int64(v)
		case int64:
			return v
		case int:
			return int64(v)
		case string:
			if v == "" {
				return nil
			}
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				return n
			}
			return nil
		}
	case core.PortTypeFloat:
		switch v := raw.(type) {
		case float64:
			return v
		case int64:
			return float64(v)
		case int:
			return float64(v)
		case string:
			if v == "" {
				return nil
			}
			if n, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				return n
			}
			return nil
		}
	case core.PortTypeBool:
		switch v := raw.(type) {
		case bool:
			return v
		case string:
			if v == "" {
				return nil
			}
			if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
				return b
			}
			return nil
		}
	case core.PortTypeString, core.PortTypeDynamic, core.PortTypeAny:
		if s, ok := raw.(string); ok {
			if s == "" {
				return nil
			}
			return s
		}
		return raw
	}
	return raw
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
