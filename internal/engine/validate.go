// 工作流 / 集合保存前的结构性校验
// 校验项：
//   - 单例节点（system_ready/update/over 等）至多 1 个
//   - 每个 exec 输出端口至多连 1 条边
//   - 每个**数据**输入端口至多 1 条入边（exec_in 多入允许，用于分支汇合）
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
	if err := validateParamRefs(asm.Nodes, asm.Params); err != nil {
		return err
	}
	if err := validateReturnRefs(asm.Nodes, asm.Returns); err != nil {
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

// validateInputSingle 每个**数据**输入端口至多 1 条入边
// exec_in 允许多入（任一上游 exec_out 推进到此处都会触发节点执行），
// 这样 branch 的 true/false 分支可以汇合回主流。与 UE Blueprint 行为一致。
// 端口类型判断沿用命名约定：以 "exec_" 开头视为 exec 端口。
func validateInputSingle(edges []core.EdgeConfig) error {
	counts := map[string]int{}
	for _, e := range edges {
		if strings.HasPrefix(e.To.Port, "exec_") {
			continue
		}
		key := e.To.Node + ":" + e.To.Port
		counts[key]++
		if counts[key] > 1 {
			return fmt.Errorf("数据输入端口 %s 只能接收 1 条边", e.To.Port)
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

// validateParamRefs assemble_param 引用的参数必须在 Params 列表中定义
func validateParamRefs(nodes []core.NodeInstance, params []core.ParamDef) error {
	defined := make(map[string]bool, len(params))
	for _, p := range params {
		defined[p.Name] = true
	}
	for _, n := range nodes {
		if n.TypeID != "assemble_param" {
			continue
		}
		nameRaw := n.Config["param_name"]
		name, _ := nameRaw.(string)
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("assemble_param 节点 %s 的 param_name 未配置", n.InstanceID)
		}
		if !defined[name] {
			return fmt.Errorf("assemble_param 节点 %s 引用的参数 %q 未定义", n.InstanceID, name)
		}
	}
	return nil
}

// validateReturnRefs return_set 引用的返回值必须在 Returns 列表中定义
func validateReturnRefs(nodes []core.NodeInstance, returns []core.ParamDef) error {
	defined := make(map[string]bool, len(returns))
	for _, r := range returns {
		defined[r.Name] = true
	}
	for _, n := range nodes {
		if n.TypeID != "return_set" {
			continue
		}
		nameRaw := n.Config["return_name"]
		name, _ := nameRaw.(string)
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("return_set 节点 %s 的 return_name 未配置", n.InstanceID)
		}
		if !defined[name] {
			return fmt.Errorf("return_set 节点 %s 引用的返回值 %q 未定义", n.InstanceID, name)
		}
	}
	return nil
}

// ── AI 草案专用校验（不挂手工保存路径：画布编辑中途保存未连完的图是正常操作） ──

// ValidateGraphConnectivity 校验图连通性：每个节点必须（沿无向边）连通到某个根节点。
// rootTypeIDs 是各独立流的入口类型——工作流为 system_ready/system_update/system_over
// （update/over 是调度器触发的独立流，合法地不连主链），集合为 assemble_start。
// 找不到任何根时从第一个节点起算，仍能拦住"散落多块"的草案。
// 仅供 AI 草案落地路径调用（手工画布允许保存未连完的图）——错误信息面向模型自我修正。
func ValidateGraphConnectivity(nodes []core.NodeInstance, edges []core.EdgeConfig, rootTypeIDs []string) error {
	if len(nodes) <= 1 {
		return nil
	}
	rootTypes := make(map[string]bool, len(rootTypeIDs))
	for _, t := range rootTypeIDs {
		rootTypes[t] = true
	}
	// 无向邻接表
	adj := make(map[string][]string, len(nodes))
	for _, e := range edges {
		adj[e.From.Node] = append(adj[e.From.Node], e.To.Node)
		adj[e.To.Node] = append(adj[e.To.Node], e.From.Node)
	}
	// 多根 BFS：从所有根类型节点同时出发
	visited := map[string]bool{}
	queue := []string{}
	for _, n := range nodes {
		if rootTypes[n.TypeID] {
			visited[n.InstanceID] = true
			queue = append(queue, n.InstanceID)
		}
	}
	if len(queue) == 0 {
		visited[nodes[0].InstanceID] = true
		queue = append(queue, nodes[0].InstanceID)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range adj[cur] {
			if !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		}
	}
	var orphans []string
	for _, n := range nodes {
		if !visited[n.InstanceID] {
			orphans = append(orphans, fmt.Sprintf("%s(%s)", n.InstanceID, n.TypeID))
		}
	}
	if len(orphans) > 0 {
		return fmt.Errorf("以下节点未通过任何边连接到执行流，请补充 edges（exec 链从入口节点逐个连到每个动作节点，数据端口如 client 也需要连线）: %s",
			strings.Join(orphans, ", "))
	}
	return nil
}

// ValidateEdgePorts 校验每条边引用的端口在节点类型定义中真实存在。
// assemble:* 引用节点的端口由 params/returns 动态生成，跳过校验。
// 仅供 AI 草案落地路径调用——错误信息附可用端口列表，便于模型修正。
func ValidateEdgePorts(nodes []core.NodeInstance, edges []core.EdgeConfig) error {
	byID := make(map[string]core.NodeInstance, len(nodes))
	for _, n := range nodes {
		byID[n.InstanceID] = n
	}
	for _, e := range edges {
		if err := checkPort(byID, e.From.Node, e.From.Port, false); err != nil {
			return err
		}
		if err := checkPort(byID, e.To.Node, e.To.Port, true); err != nil {
			return err
		}
	}
	return nil
}

// checkPort 校验单端的端口存在性。input=true 查 InputPorts，否则查 OutputPorts。
func checkPort(byID map[string]core.NodeInstance, nodeID, port string, input bool) error {
	n, ok := byID[nodeID]
	if !ok {
		return fmt.Errorf("边引用了不存在的节点: %s", nodeID)
	}
	if strings.HasPrefix(n.TypeID, "assemble:") {
		return nil
	}
	def, ok := Lookup(n.TypeID)
	if !ok {
		return nil // 类型存在性由 NodeTypeChecker 负责，这里不重复报
	}
	ports := def.TypeDef().OutputPorts
	side := "输出"
	if input {
		ports = def.TypeDef().InputPorts
		side = "输入"
	}
	available := make([]string, 0, len(ports))
	for _, p := range ports {
		if p.ID == port {
			return nil
		}
		available = append(available, p.ID)
	}
	return fmt.Errorf("节点 %s(%s) 没有%s端口 %q，可用%s端口: [%s]",
		nodeID, n.TypeID, side, port, side, strings.Join(available, ", "))
}
