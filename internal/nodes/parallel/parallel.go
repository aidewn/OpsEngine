// parallel 并发节点：触发后并行启动多个 exec 分支
// 端口：exec_in + exec_out_1..8 + exec_out_done
// 全部分支结束后才走 exec_out_done
// Phase 5 实装真正的并发逻辑；Phase 0 仅注册 TypeDef

package parallel

import (
	"fmt"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

// 并发分支端口数量（固定，UI 上未连接的端口可隐藏）
const branchCount = 8

func init() { engine.Register(&Node{}) }

// Node parallel 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	outputs := make([]core.PortDef, 0, branchCount+1)
	for i := 1; i <= branchCount; i++ {
		outputs = append(outputs, core.PortDef{
			ID:       fmt.Sprintf("exec_out_%d", i),
			Label:    fmt.Sprintf("▶ %d", i),
			PortType: core.PortTypeExec,
		})
	}
	outputs = append(outputs, core.PortDef{
		ID:       "exec_out_done",
		Label:    "▶ done",
		PortType: core.PortTypeExec,
	})

	minB, maxB := int64(2), int64(8)
	return core.NodeTypeDef{
		TypeID:      "parallel",
		DisplayName: "并发",
		Category:    "flow",
		NodeKind:    core.NodeKindFlowControl,
		Icon:        "⫴",
		Description: "并行触发多个 exec 分支，全部结束后继续",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
		},
		OutputPorts: outputs,
		ConfigSchema: []core.FieldSchema{
			{Type: "number", ID: "branch_count", Label: "分支数",
				Min: &minB, Max: &maxB, Default: int64(4)},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute Phase 0 暂时空实现，Phase 5 实装并发
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	return nil, nil
}
