// 服务器巡检的单轮处理：两阶段生成（LLM 产 Plan，后端拼工作流）。

package runtime

import (
	"errors"
	"fmt"
	"time"

	"OpsEngine/internal/agent/inspection"
	"OpsEngine/internal/agent/prompt"
	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"

	"github.com/google/uuid"
)

// resolveSSHTarget 决定本轮巡检要打到哪个 SSH 配置。
//   - 会话有 ConfigID（Scope=config 或环境级但用户偏好已记录）→ 直接用
//   - 否则环境内有唯一 SSH → 自动选定
//   - 否则报错让用户追问
func (r *Runtime) resolveSSHTarget(session core.AISession, targetConfigID string) (string, error) {
	if r.Environments == nil {
		return "", fmt.Errorf("环境查询未注入")
	}
	env, err := r.Environments(session.EnvironmentID)
	if err != nil {
		return "", err
	}
	if targetConfigID != "" {
		return inspection.PickSSHTarget(env, targetConfigID)
	}
	return inspection.PickSSHTarget(env, session.ConfigID)
}

// handleInspection 处理 intent.KindInspectServer。
func (r *Runtime) handleInspection(req Request, session core.AISession) {
	progress := []string{}
	if session.EnvironmentID == "" {
		r.emitError(req.RequestID, session.ID, "巡检需要真实环境上下文。请先在 Chat 顶部选择环境或 SSH 配置，再重新发送巡检需求。")
		return
	}

	r.emitProgress(req.RequestID, session.ID, "正在请求大模型生成巡检计划", &progress)
	systemPrompt, err := prompt.BuildInspectionPlanPrompt(prompt.InspectionInputs{
		PreferredEnvironmentID: session.EnvironmentID,
		PreferredConfigID:      session.ConfigID,
	})
	if err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}
	reply, err := r.LLM.Chat([]clients.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: req.Message},
	})
	if err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	r.emitProgress(req.RequestID, session.ID, "正在解析巡检计划", &progress)
	plan, err := inspection.ParsePlan(reply)
	if err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	// 环境级会话可能没有强制 ConfigID，需要根据环境内的 SSH 配置数自动选定或追问。
	r.emitProgress(req.RequestID, session.ID, "正在选择巡检目标 SSH", &progress)
	target, err := r.resolveSSHTarget(session, req.TargetConfigID)
	if err != nil {
		var selectErr *inspection.SSHTargetSelectionError
		if errors.As(err, &selectErr) {
			options := make([]TargetOption, 0, len(selectErr.Candidates))
			for _, c := range selectErr.Candidates {
				options = append(options, TargetOption{ID: c.ID, Name: c.Name})
			}
			if r.Emit != nil {
				r.Emit.Emit(Event{
					RequestID:     req.RequestID,
					SessionID:     session.ID,
					Type:          EventTargetSelect,
					Text:          selectErr.Error(),
					TargetOptions: options,
				})
			}
			return
		}
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	r.emitProgress(req.RequestID, session.ID, fmt.Sprintf("正在生成 %d 项巡检工作流", len(plan.Items)), &progress)
	wf, err := inspection.Materialize(plan, session.EnvironmentID, target)
	if err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	r.emitProgress(req.RequestID, session.ID, "正在保存工作流", &progress)
	if r.Workflows == nil {
		r.emitError(req.RequestID, session.ID, "工作流存储未初始化")
		return
	}
	if err := r.Workflows.Save(wf); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	r.Emit.Emit(Event{
		RequestID: req.RequestID, SessionID: session.ID,
		Type:         EventWorkflow,
		WorkflowID:   wf.ID,
		WorkflowName: wf.Name,
	})

	session.Messages = append(session.Messages, core.AISessionMessage{
		ID:           uuid.New().String(),
		Role:         core.AIMessageRoleAssistant,
		Content:      fmt.Sprintf("已生成巡检工作流「%s」，共 %d 项检查，可以直接打开执行。", wf.Name, len(plan.Items)),
		Progress:     progress,
		WorkflowID:   wf.ID,
		WorkflowName: wf.Name,
		Intent:       "inspect_server",
		CreatedAt:    time.Now(),
	})
	session.UpdatedAt = time.Now()
	if err := r.Sessions.Save(session); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}
	r.emitDone(req.RequestID, session.ID)
}
