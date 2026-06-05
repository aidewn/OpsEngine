// 通用工作流生成：让模型直接产工作流 JSON（草案）。
// 这条路径稳定性低于 inspection，仅用于无法用巡检模板覆盖的场景。
// 长期演进方向（文档 P6）是把更多场景迁到两阶段生成。

package runtime

import (
	"fmt"
	"time"

	"OpsEngine/internal/agent/prompt"
	"OpsEngine/internal/agent/workflow"
	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"

	"github.com/google/uuid"
)

// handleWorkflow 处理 intent.KindGenerateWorkflow。
func (r *Runtime) handleWorkflow(req Request, session core.AISession) {
	progress := []string{}
	r.emitProgress(req.RequestID, session.ID, "正在收集节点目录与环境信息", &progress)

	var envs []core.EnvironmentDef
	if r.EnvList != nil {
		envs, _ = r.EnvList()
	}
	var nodeTypes []core.NodeTypeDef
	if r.Nodes != nil {
		nodeTypes = r.Nodes()
	}
	systemPrompt, err := prompt.BuildWorkflowSystemPrompt(prompt.WorkflowInputs{
		NodeTypes:              nodeTypes,
		Environments:           envs,
		PreferredEnvironmentID: session.EnvironmentID,
		PreferredConfigID:      session.ConfigID,
	})
	if err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	r.emitProgress(req.RequestID, session.ID, "正在请求大模型生成工作流", &progress)
	reply, err := r.LLM.Chat([]clients.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: req.Message},
	})
	if err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	r.emitProgress(req.RequestID, session.ID, "正在解析模型返回", &progress)
	draft, err := workflow.ParseDraft(reply)
	if err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	r.emitProgress(req.RequestID, session.ID, "正在校验工作流结构", &progress)
	wf, err := workflow.Materialize(draft, workflow.NodeTypeChecker(r.NodeChecker))
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
		Content:      fmt.Sprintf("已生成工作流「%s」，可以直接打开查看。", wf.Name),
		Progress:     progress,
		WorkflowID:   wf.ID,
		WorkflowName: wf.Name,
		CreatedAt:    time.Now(),
	})
	session.UpdatedAt = time.Now()
	if err := r.Sessions.Save(session); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}
	r.emitDone(req.RequestID, session.ID)
}
