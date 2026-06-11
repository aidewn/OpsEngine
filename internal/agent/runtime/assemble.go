// 集合生成与迭代：让 Chat 可以创建/修改可复用 Assemble 资产。

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

// handleAssemble 处理 create_assemble / update_assemble。
func (r *Runtime) handleAssemble(req Request, session core.AISession, update bool) {
	progress := []string{}
	if r.Assembles == nil {
		r.emitError(req.RequestID, session.ID, "集合存储未初始化")
		return
	}
	r.emitProgress(req.RequestID, session.ID, "正在收集节点目录", &progress)

	var current core.AssembleDef
	currentJSON := ""
	if update {
		artifactID := strings.TrimSpace(req.ArtifactID)
		if artifactID == "" {
			artifactID = lastArtifactID(session, "assemble")
		}
		if artifactID == "" {
			r.emitError(req.RequestID, session.ID, "请先选择要修改的集合")
			return
		}
		var err error
		current, err = r.Assembles.Get(artifactID)
		if err != nil {
			r.emitError(req.RequestID, session.ID, err.Error())
			return
		}
		if raw, err := json.Marshal(current); err == nil {
			currentJSON = string(raw)
		}
	}

	var envs []core.EnvironmentDef
	if r.EnvList != nil {
		envs, _ = r.EnvList()
	}
	var nodeTypes []core.NodeTypeDef
	if r.Nodes != nil {
		nodeTypes = r.Nodes()
	}
	systemPrompt, err := prompt.BuildAssembleSystemPrompt(prompt.WorkflowInputs{
		NodeTypes:              nodeTypes,
		Environments:           envs,
		PreferredEnvironmentID: session.EnvironmentID,
		PreferredConfigID:      session.ConfigID,
	}, currentJSON)
	if err != nil {
		r.emitTurnError(req, &session, err.Error(), progress, assembleIntent(update))
		return
	}

	userPrompt := req.Message
	if update {
		userPrompt = fmt.Sprintf("请基于当前集合修改：%s", req.Message)
	}

	// 更新模式保留已有节点身份（diff 对齐 / 画布选中稳定），创建模式全新分配 ID。
	materialize := func(d workflow.Draft) (core.AssembleDef, error) {
		if update {
			return workflow.MaterializeAssembleUpdate(d, current, workflow.NodeTypeChecker(r.NodeChecker))
		}
		return workflow.MaterializeAssemble(d, "", workflow.NodeTypeChecker(r.NodeChecker))
	}

	intentTag := assembleIntent(update)
	draft, err := r.requestArtifactDraft(req, session.ID, &progress, systemPrompt, userPrompt,
		func(d workflow.Draft) error {
			asm, err := materialize(d)
			if err != nil {
				return err
			}
			return r.validateEnvRefs(asm.Nodes)
		},
	)
	if err != nil {
		r.emitTurnError(req, &session, err.Error(), progress, intentTag)
		return
	}
	r.emitProgress(req.RequestID, session.ID, "正在校验集合结构", &progress)
	asm, err := materialize(draft)
	if err != nil {
		r.emitTurnError(req, &session, err.Error(), progress, intentTag)
		return
	}
	if update && strings.TrimSpace(asm.Name) == "" {
		asm.Name = current.Name
	}

	r.emitProgress(req.RequestID, session.ID, "正在保存集合", &progress)
	if err := r.Assembles.Save(asm); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}
	r.emitProgress(req.RequestID, session.ID, "集合已保存", &progress)

	nodeCount := len(asm.Nodes)
	changeSummary := ""
	actionType := "create"
	content := fmt.Sprintf("已生成集合「%s」（%d 个节点），可直接打开或继续迭代修改。", asm.Name, nodeCount)
	if update {
		actionType = "update"
		changeSummary = workflow.DiffGraph(current.Nodes, asm.Nodes, current.Edges, asm.Edges).Summary()
		content = fmt.Sprintf("已更新集合「%s」（%s）。请重新运行验证；若仍失败，把新的日志贴回对话继续修复。", asm.Name, changeSummary)
	}
	if err := r.setSessionActiveArtifact(&session, "assemble", asm.ID, asm.Name); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}
	r.Emit.Emit(Event{
		RequestID: req.RequestID, SessionID: session.ID,
		Type:          EventAssemble,
		AssembleID:    asm.ID,
		AssembleName:  asm.Name,
		ArtifactType:  "assemble",
		ActionType:    actionType,
		NodeCount:     nodeCount,
		ChangeSummary: changeSummary,
	})

	session.Messages = append(session.Messages, core.AISessionMessage{
		ID:            uuid.New().String(),
		Role:          core.AIMessageRoleAssistant,
		Content:       content,
		Progress:      progress,
		AssembleID:    asm.ID,
		AssembleName:  asm.Name,
		ArtifactType:  "assemble",
		ActionType:    actionType,
		NodeCount:     nodeCount,
		ChangeSummary: changeSummary,
		Intent:        "create_assemble",
		CreatedAt:     time.Now(),
	})
	if update {
		session.Messages[len(session.Messages)-1].Intent = "update_assemble"
	}
	session.UpdatedAt = time.Now()
	if err := r.Sessions.Save(session); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}
	r.emitDone(req.RequestID, session.ID)
}

// assembleIntent 返回集合操作的 intent 标签。
func assembleIntent(update bool) string {
	if update {
		return "update_assemble"
	}
	return "create_assemble"
}

// lastArtifactID 从历史 assistant 消息中找最近的指定类型产物。
func lastArtifactID(session core.AISession, artifactType string) string {
	for i := len(session.Messages) - 1; i >= 0; i-- {
		m := session.Messages[i]
		if m.ArtifactType != artifactType {
			continue
		}
		if artifactType == "assemble" && strings.TrimSpace(m.AssembleID) != "" {
			return m.AssembleID
		}
		if artifactType == "workflow" && strings.TrimSpace(m.WorkflowID) != "" {
			return m.WorkflowID
		}
	}
	return ""
}
