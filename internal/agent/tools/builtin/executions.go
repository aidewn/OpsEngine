// 执行记录查询工具：让 Agent 在 Chat 中查看工作流执行状态与失败详情。
// 只读——执行的发起与终止仍由用户在界面操作。

package builtin

import (
	"encoding/json"
	"fmt"
	"strings"

	"OpsEngine/internal/agent/execsum"
	"OpsEngine/internal/agent/tools"
	"OpsEngine/internal/core"
)

// GetExecution 按 ID 返回执行摘要；失败执行附带失败节点的配置、日志尾部与变量快照。
type GetExecution struct{}

func (GetExecution) Spec() tools.Spec {
	return tools.Spec{
		Name: "get_execution",
		Description: "查询一次工作流执行的状态摘要。失败的执行会返回失败节点的类型、配置、日志尾部与当时变量，" +
			"用于分析失败原因或回答用户「刚才为什么失败」类问题。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"execution_id": {Type: "string", Description: "执行记录 ID。", Required: true},
		},
	}
}

func (GetExecution) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	id, err := argString(args, "execution_id")
	if err != nil {
		return tools.Result{}, err
	}
	if ctx.ExecutionGet == nil {
		return tools.Result{}, fmt.Errorf("执行记录查询未注入")
	}
	rec, err := ctx.ExecutionGet(strings.TrimSpace(id))
	if err != nil {
		return tools.Result{}, err
	}
	view := &core.AIViewPayload{Kind: "execution_summary", Title: "执行 · " + rec.Snapshot.Workflow.Name}
	if raw, mErr := json.Marshal(execsum.BuildExecutionView(rec)); mErr == nil {
		view.Data = string(raw)
	} else {
		view = nil
	}
	return tools.Result{
		Output:         tools.TruncateOutput(execsum.Summary(rec)),
		DisplaySummary: fmt.Sprintf("get_execution %s (%s)", rec.ID, rec.Status),
		View:           view,
	}, nil
}
