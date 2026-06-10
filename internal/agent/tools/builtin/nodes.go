// 节点目录与探测类 Agent 工具：把引擎内置节点能力暴露给 Chat 工具循环。
// 只读策略不变——写操作仍走「生成工作流 → 用户审查 → 执行」。

package builtin

import (
	"encoding/json"
	"fmt"
	"strings"

	"OpsEngine/internal/agent/prompt"
	"OpsEngine/internal/agent/tools"
	"OpsEngine/internal/core"
	"OpsEngine/internal/probe"
)

// ── node_catalog ──────────────────────────────────────────────

// NodeCatalog 查询内置节点目录，可按关键词/分类筛选，或按 type_id 返回完整 schema。
type NodeCatalog struct{}

func (NodeCatalog) Spec() tools.Spec {
	return tools.Spec{
		Name: "node_catalog",
		Description: "查询 OpsEngine 内置工作流节点目录（含端口、配置 schema、分类）。" +
			"编排工作流前先调用以确认 type_id 与必填 config；传 type_id 可返回单个节点完整定义。" +
			"写类节点（docker_run、linux_file_write 等）只能出现在工作流中，Chat 内不可直接执行。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"type_id":  {Type: "string", Description: "精确查询单个节点 type_id（如 env_probe_ssh_list_dir）。", Required: false},
			"query":    {Type: "string", Description: "模糊匹配 type_id / 显示名 / 描述。", Required: false},
			"category": {Type: "string", Description: "按分类过滤（environment / docker / linux / k8s / control 等）。", Required: false},
			"limit":    {Type: "integer", Description: "列表模式最多返回条数，默认 25，上限 60。", Required: false, Default: 25},
		},
	}
}

func (NodeCatalog) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	if ctx.NodeCatalog == nil {
		return tools.Result{}, fmt.Errorf("节点目录未注入")
	}
	defs := ctx.NodeCatalog()
	typeID := strings.TrimSpace(argStringOptional(args, "type_id"))
	if typeID != "" {
		for _, d := range defs {
			if d.TypeID == typeID {
				text, err := marshalPretty(prompt.SummarizeNodeTypes([]core.NodeTypeDef{d}))
				if err != nil {
					return tools.Result{}, err
				}
				return tools.Result{
					Output:         tools.TruncateOutput(text),
					DisplaySummary: "node_catalog " + typeID,
				}, nil
			}
		}
		return tools.Result{}, fmt.Errorf("未知节点类型: %s", typeID)
	}

	query := strings.ToLower(strings.TrimSpace(argStringOptional(args, "query")))
	category := strings.ToLower(strings.TrimSpace(argStringOptional(args, "category")))
	limit := argInt(args, "limit", 25)
	if limit < 1 {
		limit = 25
	}
	if limit > 60 {
		limit = 60
	}

	filtered := make([]core.NodeTypeDef, 0, len(defs))
	for _, d := range defs {
		if category != "" && !strings.EqualFold(d.Category, category) {
			continue
		}
		if query != "" {
			hay := strings.ToLower(d.TypeID + " " + d.DisplayName + " " + d.Description + " " + string(d.NodeKind))
			if !strings.Contains(hay, query) {
				continue
			}
		}
		filtered = append(filtered, d)
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	summary := prompt.SummarizeNodeTypes(filtered)
	text, err := marshalPretty(summary)
	if err != nil {
		return tools.Result{}, err
	}
	header := fmt.Sprintf("共 %d 个节点（展示 %d）", len(defs), len(summary))
	if query != "" || category != "" {
		header += fmt.Sprintf("；筛选 query=%q category=%q", query, category)
	}
	return tools.Result{
		Output:         tools.TruncateOutput(header + "\n" + text),
		DisplaySummary: fmt.Sprintf("node_catalog %d 项", len(summary)),
	}, nil
}

// ── probe_catalog ─────────────────────────────────────────────

// ProbeCatalog 列出所有可在 Chat 中调用的 env_probe_* 探测节点及参数说明。
type ProbeCatalog struct{}

func (ProbeCatalog) Spec() tools.Spec {
	return tools.Spec{
		Name: "probe_catalog",
		Description: "列出所有已注册的环境探测节点（env_probe_*），这些节点可通过 run_probe 在 Chat 中只读调用。" +
			"编排工作流时也可使用相同 type_id 作为节点。",
		Tier:   tools.TierRead,
		Params: map[string]tools.ParamSpec{},
	}
}

func (ProbeCatalog) Execute(ctx tools.ToolContext, _ map[string]any) (tools.Result, error) {
	ids := probe.ListRegisteredTypeIDs()
	if ctx.NodeCatalog != nil {
		defs := ctx.NodeCatalog()
		lines := make([]string, 0, len(ids))
		for _, id := range ids {
			desc := id
			for _, d := range defs {
				if d.TypeID == id {
					desc = fmt.Sprintf("%s — %s", id, d.DisplayName)
					break
				}
			}
			lines = append(lines, "- "+desc)
		}
		text := fmt.Sprintf("已注册探测节点 %d 个：\n%s", len(lines), strings.Join(lines, "\n"))
		return tools.Result{Output: text, DisplaySummary: fmt.Sprintf("probe_catalog %d", len(ids))}, nil
	}
	return tools.Result{
		Output:         strings.Join(ids, "\n"),
		DisplaySummary: fmt.Sprintf("probe_catalog %d", len(ids)),
	}, nil
}

// ── run_probe ─────────────────────────────────────────────────

// RunProbe 调用 env_probe_* 注册表执行只读探测，等价于编辑态「探测一次」。
type RunProbe struct{}

func (RunProbe) Spec() tools.Spec {
	return tools.Spec{
		Name: "run_probe",
		Description: "执行已注册的环境探测节点（env_probe_*）并返回结果。" +
			"node_type 必须是 probe_catalog 中的 type_id；node_config 传该节点 config 字段（如 path、pattern）。" +
			"config_id 可选，默认使用会话绑定的 SSH/配置。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"node_type":   {Type: "string", Description: "探测节点 type_id，如 env_probe_ssh_list_dir。", Required: true},
			"config_id":   {Type: "string", Description: "环境内配置 ID；省略时会话 PreferredConfigID。", Required: false},
			"node_config": {Type: "string", Description: "JSON 对象字符串，对应节点 config 字段。", Required: false, Default: "{}"},
		},
	}
}

func (RunProbe) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	nodeType, err := argString(args, "node_type")
	if err != nil {
		return tools.Result{}, err
	}
	nodeType = strings.TrimSpace(nodeType)
	if _, ok := probe.Lookup(nodeType); !ok {
		return tools.Result{}, fmt.Errorf("未注册的探测节点: %s（先用 probe_catalog 查看可用列表）", nodeType)
	}
	if ctx.EnvLookup == nil || strings.TrimSpace(ctx.EnvironmentID) == "" {
		return tools.Result{}, fmt.Errorf("当前会话未绑定环境，无法执行探测")
	}
	env, err := ctx.EnvLookup(ctx.EnvironmentID)
	if err != nil {
		return tools.Result{}, err
	}
	configID := strings.TrimSpace(argStringOptional(args, "config_id"))
	if configID == "" {
		configID = strings.TrimSpace(ctx.PreferredConfigID)
	}
	if configID == "" {
		return tools.Result{}, fmt.Errorf("请指定 config_id 或在会话中绑定 SSH 配置")
	}

	nodeConfig := map[string]any{}
	rawCfg := strings.TrimSpace(argStringOptional(args, "node_config"))
	if rawCfg != "" && rawCfg != "{}" {
		if err := json.Unmarshal([]byte(rawCfg), &nodeConfig); err != nil {
			return tools.Result{}, fmt.Errorf("node_config 必须是 JSON 对象: %w", err)
		}
	}

	res, err := probe.Run(nodeType, env, configID, nodeConfig)
	if err != nil {
		return tools.Result{}, err
	}
	text := formatProbeResult(nodeType, res)
	return tools.Result{
		Output:         tools.TruncateOutput(text),
		DisplaySummary: fmt.Sprintf("run_probe %s (%d 项)", nodeType, len(res.Items)),
	}, nil
}

// ── list_workflows / get_workflow ─────────────────────────────

type ListWorkflows struct{}

func (ListWorkflows) Spec() tools.Spec {
	return tools.Spec{
		Name:        "list_workflows",
		Description: "列出本地已保存的工作流 id 与名称，用于迭代修改前定位目标。",
		Tier:        tools.TierRead,
		Params:      map[string]tools.ParamSpec{},
	}
}

func (ListWorkflows) Execute(ctx tools.ToolContext, _ map[string]any) (tools.Result, error) {
	if ctx.WorkflowList == nil {
		return tools.Result{}, fmt.Errorf("工作流存储未注入")
	}
	items, err := ctx.WorkflowList()
	if err != nil {
		return tools.Result{}, err
	}
	lines := make([]string, 0, len(items))
	for _, wf := range items {
		lines = append(lines, fmt.Sprintf("- %s · %s（%d 节点）", wf.ID, wf.Name, len(wf.Nodes)))
	}
	text := fmt.Sprintf("工作流 %d 个：\n%s", len(lines), strings.Join(lines, "\n"))
	return tools.Result{Output: tools.TruncateOutput(text), DisplaySummary: fmt.Sprintf("list_workflows %d", len(items))}, nil
}

type GetWorkflow struct{}

func (GetWorkflow) Spec() tools.Spec {
	return tools.Spec{
		Name:        "get_workflow",
		Description: "获取指定工作流的结构摘要（节点 type、数量、变量），不含完整坐标。修改工作流前先调用了解现状。",
		Tier:        tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"workflow_id": {Type: "string", Description: "工作流 ID。", Required: true},
		},
	}
}

func (GetWorkflow) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	id, err := argString(args, "workflow_id")
	if err != nil {
		return tools.Result{}, err
	}
	if ctx.WorkflowGet == nil {
		return tools.Result{}, fmt.Errorf("工作流存储未注入")
	}
	wf, err := ctx.WorkflowGet(strings.TrimSpace(id))
	if err != nil {
		return tools.Result{}, err
	}
	text := summarizeWorkflow(wf)
	return tools.Result{Output: tools.TruncateOutput(text), DisplaySummary: "get_workflow " + wf.Name}, nil
}

// ── list_assembles / get_assemble ─────────────────────────────

type ListAssembles struct{}

func (ListAssembles) Spec() tools.Spec {
	return tools.Spec{
		Name:        "list_assembles",
		Description: "列出本地已保存的集合 id 与名称。",
		Tier:        tools.TierRead,
		Params:      map[string]tools.ParamSpec{},
	}
}

func (ListAssembles) Execute(ctx tools.ToolContext, _ map[string]any) (tools.Result, error) {
	if ctx.AssembleList == nil {
		return tools.Result{}, fmt.Errorf("集合存储未注入")
	}
	items, err := ctx.AssembleList()
	if err != nil {
		return tools.Result{}, err
	}
	lines := make([]string, 0, len(items))
	for _, asm := range items {
		lines = append(lines, fmt.Sprintf("- %s · %s（%d 节点）", asm.ID, asm.Name, len(asm.Nodes)))
	}
	text := fmt.Sprintf("集合 %d 个：\n%s", len(lines), strings.Join(lines, "\n"))
	return tools.Result{Output: tools.TruncateOutput(text), DisplaySummary: fmt.Sprintf("list_assembles %d", len(items))}, nil
}

type GetAssemble struct{}

func (GetAssemble) Spec() tools.Spec {
	return tools.Spec{
		Name:        "get_assemble",
		Description: "获取指定集合的结构摘要（参数、节点 type、数量）。",
		Tier:        tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"assemble_id": {Type: "string", Description: "集合 ID。", Required: true},
		},
	}
}

func (GetAssemble) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	id, err := argString(args, "assemble_id")
	if err != nil {
		return tools.Result{}, err
	}
	if ctx.AssembleGet == nil {
		return tools.Result{}, fmt.Errorf("集合存储未注入")
	}
	asm, err := ctx.AssembleGet(strings.TrimSpace(id))
	if err != nil {
		return tools.Result{}, err
	}
	text := summarizeAssemble(asm)
	return tools.Result{Output: tools.TruncateOutput(text), DisplaySummary: "get_assemble " + asm.Name}, nil
}

// ── 辅助 ─────────────────────────────────────────────────────

func marshalPretty(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func formatProbeResult(nodeType string, res probe.ProbeResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "probe %s: %d items\n", nodeType, len(res.Items))
	for i, item := range res.Items {
		if i >= 80 {
			b.WriteString("...(结果已截断)\n")
			break
		}
		fmt.Fprintf(&b, "- %s", item.Key)
		if item.Label != "" && item.Label != item.Key {
			fmt.Fprintf(&b, " (%s)", item.Label)
		}
		if len(item.Meta) > 0 {
			meta, _ := json.Marshal(item.Meta)
			fmt.Fprintf(&b, " %s", string(meta))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func summarizeWorkflow(wf core.WorkflowDef) string {
	var b strings.Builder
	fmt.Fprintf(&b, "工作流 %s · %s\n", wf.ID, wf.Name)
	if wf.Description != "" {
		fmt.Fprintf(&b, "描述: %s\n", wf.Description)
	}
	fmt.Fprintf(&b, "节点 %d · 边 %d · 变量 %d\n", len(wf.Nodes), len(wf.Edges), len(wf.Variables))
	typeCount := map[string]int{}
	for _, n := range wf.Nodes {
		typeCount[n.TypeID]++
	}
	b.WriteString("节点类型:\n")
	for typ, cnt := range typeCount {
		fmt.Fprintf(&b, "  - %s × %d\n", typ, cnt)
	}
	return b.String()
}

func summarizeAssemble(asm core.AssembleDef) string {
	var b strings.Builder
	fmt.Fprintf(&b, "集合 %s · %s\n", asm.ID, asm.Name)
	if asm.Description != "" {
		fmt.Fprintf(&b, "描述: %s\n", asm.Description)
	}
	fmt.Fprintf(&b, "参数 %d · 节点 %d · 边 %d\n", len(asm.Params), len(asm.Nodes), len(asm.Edges))
	if len(asm.Params) > 0 {
		b.WriteString("参数: ")
		for i, p := range asm.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(p.Name)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
