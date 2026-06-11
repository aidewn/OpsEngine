// 工作流生成/修改：进入带 propose 工具的会话循环（计划书 P4）。
// 模型先用只读工具核实环境事实，再调用 propose_workflow / propose_update_workflow 提交草案；
// 校验失败的错误经工具消息反馈给模型自行修正。
// runWorkflowUpdateTurn 保留给 execfix——结构化失败修复有完整上下文，一次性生成更稳。

package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"OpsEngine/internal/agent/prompt"
	"OpsEngine/internal/agent/workflow"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	"github.com/google/uuid"
)

// validateEnvRefs 校验落地节点的环境/环境配置引用存在性；未注入环境查询时跳过。
func (r *Runtime) validateEnvRefs(nodes []core.NodeInstance) error {
	if r.Environments == nil {
		return nil
	}
	return engine.ValidateEnvRefs(nodes, engine.EnvLookup(r.Environments))
}

// handleWorkflow 处理 intent.KindGenerateWorkflow：带编排引导段进入工具循环。
// 落盘动作由模型调用 propose_workflow 触发（proposal.go），本函数只负责注入任务引导。
func (r *Runtime) handleWorkflow(req Request, session core.AISession) {
	r.runConversationTurn(req, session, turnOpts{
		SystemKind:  prompt.SystemPromptChat,
		IntentTag:   "generate_workflow",
		ExtraSystem: buildGenerationGuidance(""),
		ToolProfile: toolProfileWorkflow,
	})
}

// handleWorkflowUpdate 处理已有工作流的 AI 修改：定位目标后带当前 JSON 进入工具循环。
// 落盘动作由模型调用 propose_update_workflow 触发，确认模式下进入待确认草案。
func (r *Runtime) handleWorkflowUpdate(req Request, session core.AISession) {
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
	currentJSON, _ := json.Marshal(existing)
	r.runConversationTurn(req, session, turnOpts{
		SystemKind:  prompt.SystemPromptChat,
		IntentTag:   "update_workflow",
		ExtraSystem: buildGenerationGuidance(string(currentJSON)),
		ToolProfile: toolProfileWorkflow,
	})
}

// runWorkflowUpdateTurn 执行一次「基于已有工作流的 AI 更新」回合：
// 构建 system prompt → 请求草案（带重试）→ 落地校验 → 保存 → 事件与会话回写。
// handleWorkflowUpdate（用户口头修改）与 handleExecutionFix（执行失败修复）共用。
func (r *Runtime) runWorkflowUpdateTurn(
	req Request,
	session core.AISession,
	existing core.WorkflowDef,
	userPrompt string,
	intentTag string,
	progress []string,
) {
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

	draft, err := r.requestArtifactDraft(req, session.ID, &progress, systemPrompt, userPrompt,
		func(d workflow.Draft) error {
			wf, err := workflow.MaterializeWorkflowUpdate(d, existing, workflow.NodeTypeChecker(r.NodeChecker))
			if err != nil {
				return err
			}
			return r.validateEnvRefs(wf.Nodes)
		},
	)
	if err != nil {
		r.emitTurnError(req, &session, err.Error(), progress, intentTag)
		return
	}
	r.emitProgress(req.RequestID, session.ID, "正在校验工作流结构", &progress)
	wf, err := workflow.MaterializeWorkflowUpdate(draft, existing, workflow.NodeTypeChecker(r.NodeChecker))
	if err != nil {
		r.emitTurnError(req, &session, err.Error(), progress, intentTag)
		return
	}
	// 确认模式：不落盘，暂存草案等用户在对话中点「应用」
	if r.ApplyMode == ApplyModeConfirm {
		r.stagePendingWorkflow(req, session, existing, wf, intentTag, progress)
		return
	}
	r.emitProgress(req.RequestID, session.ID, "正在保存工作流", &progress)
	if err := r.Workflows.Save(wf); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}
	nodeCount := len(wf.Nodes)
	changeSummary := workflow.DiffWorkflows(existing, wf).Summary()
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
		Intent:        intentTag,
		CreatedAt:     time.Now(),
	})
	session.UpdatedAt = time.Now()
	if err := r.Sessions.Save(session); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}
	r.emitDone(req.RequestID, session.ID)
}
