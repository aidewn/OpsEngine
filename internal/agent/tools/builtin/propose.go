// propose 类写工具：模型在工具循环内提交工作流草案，由 runtime 注入的回调完成
// 解析 → 校验（结构 + config schema + 环境引用）→ 落盘（或确认模式下暂存草案）。
// 这是注册表里仅有的 low_write 工具；校验失败的错误文本原样返回给模型自行修正重试。

package builtin

import (
	"fmt"

	"OpsEngine/internal/agent/tools"
)

// draftJSONDesc 是两个工具共用的 draft_json 参数说明。
const draftJSONDesc = "完整工作流草案 JSON：{name, description, variables, nodes:[{id, type_id, config, position:{x,y}}], edges:[{from:{node,port}, to:{node,port}}]}。" +
	"节点 id 用 n1/n2 等占位；type_id 与 config 必须符合 node_catalog 返回的 schema。"

// ── propose_workflow ──────────────────────────────────────────

// ProposeWorkflow 创建新工作流。
type ProposeWorkflow struct{}

func (ProposeWorkflow) Spec() tools.Spec {
	return tools.Spec{
		Name: "propose_workflow",
		Description: "把设计好的工作流草案提交保存为新工作流。提交前必须用 node_catalog 确认 type_id 与必填 config，" +
			"涉及真实路径/服务名时先用只读工具核实。校验失败会返回具体错误（含节点与字段），修正后重试。" +
			"草案必须基于已核实的事实，禁止凭空编造路径。",
		Tier: tools.TierLowWrite,
		Params: map[string]tools.ParamSpec{
			"draft_json": {Type: "string", Description: draftJSONDesc, Required: true},
		},
	}
}

func (ProposeWorkflow) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	draftJSON, err := argString(args, "draft_json")
	if err != nil {
		return tools.Result{}, err
	}
	if ctx.ProposeWorkflow == nil {
		return tools.Result{}, fmt.Errorf("当前会话未启用工作流提交能力")
	}
	res, err := ctx.ProposeWorkflow(draftJSON)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{
		Output: fmt.Sprintf("工作流已创建：id=%s name=%s 节点=%d。请向用户简要总结工作流内容（不要贴 JSON）。",
			res.WorkflowID, res.WorkflowName, res.NodeCount),
		DisplaySummary: fmt.Sprintf("propose_workflow %s（%d 节点）", res.WorkflowName, res.NodeCount),
	}, nil
}

// ── propose_update_workflow ───────────────────────────────────

// ProposeUpdateWorkflow 更新已有工作流（确认模式下生成待确认草案）。
type ProposeUpdateWorkflow struct{}

func (ProposeUpdateWorkflow) Spec() tools.Spec {
	return tools.Spec{
		Name: "propose_update_workflow",
		Description: "把修改后的完整工作流草案提交为已有工作流的更新。未修改的节点必须原样回显其 instance_id（保持节点身份与 diff 对齐）。" +
			"系统可能配置为确认模式：此时草案进入待确认状态，由用户在界面点「应用」后才生效。",
		Tier: tools.TierLowWrite,
		Params: map[string]tools.ParamSpec{
			"workflow_id": {Type: "string", Description: "要更新的工作流 ID。", Required: true},
			"draft_json":  {Type: "string", Description: draftJSONDesc, Required: true},
		},
	}
}

func (ProposeUpdateWorkflow) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	workflowID, err := argString(args, "workflow_id")
	if err != nil {
		return tools.Result{}, err
	}
	draftJSON, err := argString(args, "draft_json")
	if err != nil {
		return tools.Result{}, err
	}
	if ctx.ProposeWorkflowUpdate == nil {
		return tools.Result{}, fmt.Errorf("当前会话未启用工作流更新能力")
	}
	res, err := ctx.ProposeWorkflowUpdate(workflowID, draftJSON)
	if err != nil {
		return tools.Result{}, err
	}
	if res.Pending {
		return tools.Result{
			Output: fmt.Sprintf("更新草案已生成并等待用户确认（%s）。请告知用户在对话下方的卡片中点「应用」生效，或「放弃」。",
				res.ChangeSummary),
			DisplaySummary: fmt.Sprintf("propose_update %s（待确认：%s）", res.WorkflowName, res.ChangeSummary),
		}, nil
	}
	return tools.Result{
		Output: fmt.Sprintf("工作流已更新：id=%s name=%s（%s）。请向用户简要总结改动。",
			res.WorkflowID, res.WorkflowName, res.ChangeSummary),
		DisplaySummary: fmt.Sprintf("propose_update %s（%s）", res.WorkflowName, res.ChangeSummary),
	}, nil
}
