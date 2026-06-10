// 集合生成与迭代：让 Chat 可以创建/修改可复用 Assemble 资产。

package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"OpsEngine/internal/agent/prompt"
	"OpsEngine/internal/agent/workflow"
	"OpsEngine/internal/clients"
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
	fixedID := ""
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
		fixedID = current.ID
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
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	r.emitProgress(req.RequestID, session.ID, "正在请求大模型生成集合", &progress)
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
	r.emitProgress(req.RequestID, session.ID, "正在校验集合结构", &progress)
	asm, err := workflow.MaterializeAssemble(draft, fixedID, workflow.NodeTypeChecker(r.NodeChecker))
	if err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
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

	actionType := "create"
	content := fmt.Sprintf("已生成集合「%s」，可以直接打开查看。", asm.Name)
	if update {
		actionType = "update"
		content = fmt.Sprintf("已更新集合「%s」。", asm.Name)
	}
	r.Emit.Emit(Event{
		RequestID: req.RequestID, SessionID: session.ID,
		Type:         EventAssemble,
		AssembleID:   asm.ID,
		AssembleName: asm.Name,
		ArtifactType: "assemble",
		ActionType:   actionType,
	})

	session.Messages = append(session.Messages, core.AISessionMessage{
		ID:           uuid.New().String(),
		Role:         core.AIMessageRoleAssistant,
		Content:      content,
		Progress:     progress,
		AssembleID:   asm.ID,
		AssembleName: asm.Name,
		ArtifactType: "assemble",
		ActionType:   actionType,
		Intent:       "create_assemble",
		CreatedAt:    time.Now(),
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
