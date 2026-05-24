// 工作流 / 集合保存前的结构性校验
// 校验项：
//   - 单例节点（system_ready/update/over 等）至多 1 个
//   - 每个 exec 输出端口至多连 1 条边

package engine

import (
	"fmt"
	"strings"

	"OpsEngine/internal/core"
)

// 工作流中受单例约束的节点类型
var workflowSingletonTypes = []string{
	"system_ready",
	"system_update",
	"system_over",
}

// 集合中受单例约束的节点类型
var assembleSingletonTypes = []string{
	"assemble_start",
	"assemble_end",
}

// ValidateWorkflow 工作流保存前的合法性校验
func ValidateWorkflow(wf core.WorkflowDef) error {
	if err := validateSingletons(wf.Nodes, workflowSingletonTypes); err != nil {
		return err
	}
	if err := validateExecOutSingle(wf.Edges); err != nil {
		return err
	}
	return nil
}

// ValidateAssemble 集合保存前的合法性校验
func ValidateAssemble(asm core.AssembleDef) error {
	if err := validateSingletons(asm.Nodes, assembleSingletonTypes); err != nil {
		return err
	}
	if err := validateExecOutSingle(asm.Edges); err != nil {
		return err
	}
	return nil
}

// validateSingletons 单例节点至多 1 个
func validateSingletons(nodes []core.NodeInstance, types []string) error {
	for _, typeID := range types {
		count := 0
		for _, n := range nodes {
			if n.TypeID == typeID {
				count++
			}
		}
		if count > 1 {
			return fmt.Errorf("节点类型 %s 只能存在 1 个，当前 %d 个", typeID, count)
		}
	}
	return nil
}

// validateExecOutSingle 每个 exec 输出端口至多 1 条出边
// 约束依赖端口命名约定：所有 exec 端口 ID 以 "exec_" 开头
// 这样不需要查每个节点的 TypeDef，集合调用节点（assemble:<id>）也能统一校验
func validateExecOutSingle(edges []core.EdgeConfig) error {
	counts := map[string]int{}
	for _, e := range edges {
		if !strings.HasPrefix(e.From.Port, "exec_") {
			continue
		}
		key := e.From.Node + ":" + e.From.Port
		counts[key]++
		if counts[key] > 1 {
			return fmt.Errorf("exec 输出端口 %s 只能连 1 条线", e.From.Port)
		}
	}
	return nil
}
