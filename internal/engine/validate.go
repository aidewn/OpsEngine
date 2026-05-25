// 工作流 / 集合保存前的结构性校验
// 校验项：
//   - 单例节点（system_ready/update/over 等）至多 1 个
//   - 每个 exec 输出端口至多连 1 条边
//   - 每个输入端口至多 1 条入边
//   - var_set / var_get 引用的变量必须在 Variables 列表中定义

package engine

import (
	"fmt"
	"strings"

	"OpsEngine/internal/core"
)

// 引用变量的节点类型，对应配置字段固定为 "var_name"
var variableRefTypes = map[string]bool{
	"var_set": true,
	"var_get": true,
}

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
	if err := validateInputSingle(wf.Edges); err != nil {
		return err
	}
	if err := validateVariableRefs(wf.Nodes, wf.Variables); err != nil {
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
	if err := validateInputSingle(asm.Edges); err != nil {
		return err
	}
	if err := validateVariableRefs(asm.Nodes, asm.Variables); err != nil {
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

// validateInputSingle 每个输入端口至多 1 条入边
// 适用所有 input（exec_in / 数据 input），参考 UE Blueprint 单入语义
func validateInputSingle(edges []core.EdgeConfig) error {
	counts := map[string]int{}
	for _, e := range edges {
		key := e.To.Node + ":" + e.To.Port
		counts[key]++
		if counts[key] > 1 {
			return fmt.Errorf("输入端口 %s 只能接收 1 条边", e.To.Port)
		}
	}
	return nil
}

// validateVariableRefs var_set / var_get 引用的变量必须在 Variables 列表中定义
// 防止用户删除变量后保存留下悬空引用
func validateVariableRefs(nodes []core.NodeInstance, variables []core.VariableDef) error {
	defined := make(map[string]bool, len(variables))
	for _, v := range variables {
		defined[v.Name] = true
	}
	for _, n := range nodes {
		if !variableRefTypes[n.TypeID] {
			continue
		}
		nameRaw := n.Config["var_name"]
		name, _ := nameRaw.(string)
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("%s 节点 %s 的 var_name 未配置", n.TypeID, n.InstanceID)
		}
		if !defined[name] {
			return fmt.Errorf("%s 节点 %s 引用的变量 %q 未定义", n.TypeID, n.InstanceID, name)
		}
	}
	return nil
}
