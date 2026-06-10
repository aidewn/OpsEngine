// 通用工作流生成：让模型直接产工作流 JSON（草案）。
// 这条路径稳定性低于 inspection，仅用于无法用巡检模板覆盖的场景。
// 长期演进方向（文档 P6）是把更多场景迁到两阶段生成。

package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"OpsEngine/internal/agent/prompt"
	"OpsEngine/internal/agent/workflow"
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
		r.emitTurnError(req, &session, err.Error(), progress, "generate_workflow")
		return
	}

	draft, err := r.requestArtifactDraft(req, session.ID, &progress, systemPrompt, req.Message,
		func(d workflow.Draft) error {
			_, err := workflow.Materialize(d, workflow.NodeTypeChecker(r.NodeChecker))
			return err
		},
	)
	if err != nil {
		r.emitTurnError(req, &session, err.Error(), progress, "generate_workflow")
		return
	}

	r.emitProgress(req.RequestID, session.ID, "正在校验工作流结构", &progress)
	wf, err := workflow.Materialize(draft, workflow.NodeTypeChecker(r.NodeChecker))
	if err != nil {
		r.emitTurnError(req, &session, err.Error(), progress, "generate_workflow")
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

	nodeCount := len(wf.Nodes)
	if err := r.setSessionActiveArtifact(&session, "workflow", wf.ID, wf.Name); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	r.Emit.Emit(Event{
		RequestID: req.RequestID, SessionID: session.ID,
		Type:         EventWorkflow,
		WorkflowID:   wf.ID,
		WorkflowName: wf.Name,
		ArtifactType: "workflow",
		ActionType:   "create",
		NodeCount:    nodeCount,
	})

	session.Messages = append(session.Messages, core.AISessionMessage{
		ID:           uuid.New().String(),
		Role:         core.AIMessageRoleAssistant,
		Content:      fmt.Sprintf("已生成工作流「%s」（%d 个节点），可直接打开查看或继续在此对话中迭代修改。", wf.Name, nodeCount),
		Progress:     progress,
		WorkflowID:   wf.ID,
		WorkflowName: wf.Name,
		ArtifactType: "workflow",
		ActionType:   "create",
		NodeCount:    nodeCount,
		Intent:       "generate_workflow",
		CreatedAt:    time.Now(),
	})
	session.UpdatedAt = time.Now()
	if err := r.Sessions.Save(session); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}
	r.emitDone(req.RequestID, session.ID)
}

// handleWorkflowUpdate 处理已有工作流的 AI 直接修改。
func (r *Runtime) handleWorkflowUpdate(req Request, session core.AISession) {
	progress := []string{}
	if r.Workflows == nil {
		r.emitError(req.RequestID, session.ID, "工作流存储未初始化")
		return
	}
	artifactID := strings.TrimSpace(req.ArtifactID)
	if artifactID == "" {
		artifactID = lastArtifactID(session, "workflow")
	}
	if artifactID == "" {
		r.emitError(req.RequestID, session.ID, "请先选择要修改的工作流")
		return
	}
	existing, err := r.Workflows.Get(artifactID)
	if err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	r.emitProgress(req.RequestID, session.ID, "正在收集节点目录与当前工作流", &progress)
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
	currentJSON, _ := json.Marshal(existing)
	userPrompt := fmt.Sprintf("请基于当前工作流 JSON 直接输出完整更新后的工作流草案 JSON。\n当前工作流：%s\n修改要求：%s", string(currentJSON), req.Message)
	if isExecutionFailureReport(req.Message) {
		r.emitProgress(req.RequestID, session.ID, "检测到执行失败日志，正在分析并修复…", &progress)
		userPrompt = buildExecutionFixUserPrompt(req.Message)
	}

	draft, err := r.requestArtifactDraft(req, session.ID, &progress, systemPrompt, userPrompt,
		func(d workflow.Draft) error {
			_, err := workflow.MaterializeWorkflowUpdate(d, existing, workflow.NodeTypeChecker(r.NodeChecker))
			return err
		},
	)
	if err != nil {
		r.emitTurnError(req, &session, err.Error(), progress, "update_workflow")
		return
	}
	r.emitProgress(req.RequestID, session.ID, "正在校验工作流结构", &progress)
	wf, err := workflow.MaterializeWorkflowUpdate(draft, existing, workflow.NodeTypeChecker(r.NodeChecker))
	if err != nil {
		r.emitTurnError(req, &session, err.Error(), progress, "update_workflow")
		return
	}
	r.emitProgress(req.RequestID, session.ID, "正在保存工作流", &progress)
	if err := r.Workflows.Save(wf); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}
	nodeCount := len(wf.Nodes)
	changeSummary := workflowChangeSummary(len(existing.Nodes), nodeCount)
	if err := r.setSessionActiveArtifact(&session, "workflow", wf.ID, wf.Name); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}
	r.Emit.Emit(Event{
		RequestID: req.RequestID, SessionID: session.ID,
		Type:          EventWorkflow,
		WorkflowID:    wf.ID,
		WorkflowName:  wf.Name,
		ArtifactType:  "workflow",
		ActionType:    "update",
		NodeCount:     nodeCount,
		ChangeSummary: changeSummary,
	})
	session.Messages = append(session.Messages, core.AISessionMessage{
		ID:            uuid.New().String(),
		Role:          core.AIMessageRoleAssistant,
		Content:       fmt.Sprintf("已更新工作流「%s」（%s）。请重新运行验证；若仍失败，把新的日志贴回对话继续修复。", wf.Name, changeSummary),
		Progress:      progress,
		WorkflowID:    wf.ID,
		WorkflowName:  wf.Name,
		ArtifactType:  "workflow",
		ActionType:    "update",
		NodeCount:     nodeCount,
		ChangeSummary: changeSummary,
		Intent:        "update_workflow",
		CreatedAt:     time.Now(),
	})
	session.UpdatedAt = time.Now()
	if err := r.Sessions.Save(session); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}
	r.emitDone(req.RequestID, session.ID)
}
