// 执行失败修复：根据 ExecutionRecord 的结构化失败信息修复工作流。
// 入口只有前端显式 operation=fix_execution（执行详情页「AI 修复」按钮），不做关键词推断。

package runtime

import (
	"encoding/json"
	"fmt"

	"OpsEngine/internal/agent/execsum"
	"OpsEngine/internal/core"
)

// handleExecutionFix 处理 intent.KindFixExecution。
// 失败上下文取自执行快照（精确还原失败现场），修复基线取当前存储的工作流
// （用户可能在执行后手工调整过，不能用快照覆盖这些修改）。
func (r *Runtime) handleExecutionFix(req Request, session core.AISession) {
	progress := []string{}
	if req.ExecutionID == "" {
		r.emitError(req.RequestID, session.ID, "缺少 execution_id，无法定位要修复的执行")
		return
	}
	if r.Executions == nil {
		r.emitError(req.RequestID, session.ID, "执行记录查询未初始化")
		return
	}
	if r.Workflows == nil {
		r.emitError(req.RequestID, session.ID, "工作流存储未初始化")
		return
	}

	r.emitProgress(req.RequestID, session.ID, "正在读取执行记录", &progress)
	rec, err := r.Executions(req.ExecutionID)
	if err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}
	if rec.Status != core.WorkflowStatusFailed && rec.Status != core.WorkflowStatusTerminated {
		r.emitError(req.RequestID, session.ID,
			fmt.Sprintf("执行 %s 状态为 %s，仅失败/终止的执行支持 AI 修复", rec.ID, rec.Status))
		return
	}
	existing, err := r.Workflows.Get(rec.WorkflowID)
	if err != nil {
		r.emitError(req.RequestID, session.ID, "执行对应的工作流已不存在: "+err.Error())
		return
	}

	r.emitProgress(req.RequestID, session.ID, "正在提取失败节点与日志", &progress)
	userPrompt := buildExecutionFixPrompt(rec, existing, req.Message)
	r.runWorkflowUpdateTurn(req, session, existing, userPrompt, "fix_execution", progress)
}

// buildExecutionFixPrompt 组装修复回合的用户提示：当前工作流 JSON + 结构化失败上下文 + 用户补充说明。
func buildExecutionFixPrompt(rec core.ExecutionRecord, existing core.WorkflowDef, userNote string) string {
	currentJSON, _ := json.Marshal(existing)
	prompt := fmt.Sprintf(`工作流执行失败，请根据下面的结构化失败信息修复工作流，输出完整更新后的工作流草案 JSON。

当前工作流：%s

%s
修复要求：
- 先根据失败节点的日志与配置定位根因，再修改对应节点，不要无关改动
- 脚本类失败重点检查 sed/awk 转义、变量引用、shell 语法
- 保持工作流的参数与整体结构不变，除非失败信息表明必须调整
- 只输出 JSON，禁止 Markdown 与解释文字`, string(currentJSON), execsum.FailureContext(rec))
	if userNote != "" {
		prompt += "\n\n用户补充说明：" + userNote
	}
	return prompt
}
