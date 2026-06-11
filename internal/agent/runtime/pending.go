// AI 修改草案的确认模式：暂存（stage）→ 用户应用（Apply）/ 放弃（Discard）。
// 设计要点（计划书 P3 第二步）：
//   - 草案存在会话上（AISession.PendingDraft），刷新/重开应用不丢
//   - 应用前用 BaseHash 检测基线漂移：用户在草案生成后手工改过工作流则拒绝应用
//   - 仅 update/fix 路径走确认；create 没有覆盖风险，始终直接保存

package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"OpsEngine/internal/agent/workflow"
	"OpsEngine/internal/core"

	"github.com/google/uuid"
)

// ApplyModeConfirm 是 Runtime.ApplyMode 的确认模式取值。
const ApplyModeConfirm = "confirm"

// workflowHash 计算工作流 JSON 的 SHA-256，作为基线指纹。
func workflowHash(wf core.WorkflowDef) string {
	raw, _ := json.Marshal(wf)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// stagePendingWorkflow 把校验通过的更新草案暂存到会话，推送 workflow_pending 事件。
func (r *Runtime) stagePendingWorkflow(
	req Request,
	session core.AISession,
	existing core.WorkflowDef,
	wf core.WorkflowDef,
	intentTag string,
	progress []string,
) {
	diff := workflow.DiffWorkflows(existing, wf)
	changeSummary := diff.Summary()
	draftJSON, err := json.Marshal(wf)
	if err != nil {
		r.emitError(req.RequestID, session.ID, "草案序列化失败: "+err.Error())
		return
	}
	session.PendingDraft = &core.AIPendingDraft{
		ArtifactType:  "workflow",
		ArtifactID:    wf.ID,
		ArtifactName:  wf.Name,
		DraftJSON:     string(draftJSON),
		BaseHash:      workflowHash(existing),
		ChangeSummary: changeSummary,
		CreatedAt:     time.Now(),
	}
	r.emitProgress(req.RequestID, session.ID, "修改草案已生成，等待确认", &progress)
	r.Emit.Emit(Event{
		RequestID: req.RequestID, SessionID: session.ID,
		Type:          EventWorkflowPending,
		WorkflowID:    wf.ID,
		WorkflowName:  wf.Name,
		ArtifactType:  "workflow",
		ActionType:    "pending",
		NodeCount:     len(wf.Nodes),
		ChangeSummary: changeSummary,
	})
	session.Messages = append(session.Messages, core.AISessionMessage{
		ID:            uuid.New().String(),
		Role:          core.AIMessageRoleAssistant,
		Content:       fmt.Sprintf("已生成「%s」的修改草案（%s）。请确认后应用，或放弃本次修改。", wf.Name, changeSummary),
		Progress:      progress,
		WorkflowID:    wf.ID,
		WorkflowName:  wf.Name,
		ArtifactType:  "workflow",
		ActionType:    "pending",
		NodeCount:     len(wf.Nodes),
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

// ApplyPendingDraft 把会话中的待确认草案落盘。
// 基线漂移（草案生成后工作流被手工修改）时返回错误并保留草案，由用户决定重新生成或放弃。
func (r *Runtime) ApplyPendingDraft(sessionID string) (core.WorkflowDef, error) {
	if r.Sessions == nil || r.Workflows == nil {
		return core.WorkflowDef{}, errors.New("依赖未初始化")
	}
	session, err := r.Sessions.Get(sessionID)
	if err != nil {
		return core.WorkflowDef{}, err
	}
	pending := session.PendingDraft
	if pending == nil {
		return core.WorkflowDef{}, errors.New("当前会话没有待确认的修改草案")
	}
	var wf core.WorkflowDef
	if err := json.Unmarshal([]byte(pending.DraftJSON), &wf); err != nil {
		return core.WorkflowDef{}, fmt.Errorf("草案解析失败: %w", err)
	}
	current, err := r.Workflows.Get(pending.ArtifactID)
	if err != nil {
		return core.WorkflowDef{}, fmt.Errorf("草案对应的工作流已不存在: %w", err)
	}
	if workflowHash(current) != pending.BaseHash {
		return core.WorkflowDef{}, errors.New("工作流在草案生成后被修改过，为避免覆盖请放弃草案并重新发起 AI 修改")
	}
	if err := r.Workflows.Save(wf); err != nil {
		return core.WorkflowDef{}, err
	}
	r.finishPendingDraft(session, fmt.Sprintf("已应用修改草案：「%s」（%s）。", wf.Name, pending.ChangeSummary))
	return wf, nil
}

// DiscardPendingDraft 放弃会话中的待确认草案。
func (r *Runtime) DiscardPendingDraft(sessionID string) error {
	if r.Sessions == nil {
		return errors.New("会话存储未初始化")
	}
	session, err := r.Sessions.Get(sessionID)
	if err != nil {
		return err
	}
	if session.PendingDraft == nil {
		return nil // 幂等：没有草案视为已放弃
	}
	name := session.PendingDraft.ArtifactName
	r.finishPendingDraft(session, fmt.Sprintf("已放弃「%s」的修改草案，工作流保持原样。", name))
	return nil
}

// finishPendingDraft 清除草案并追加一条结果消息（保存失败仅静默——主操作已完成）。
func (r *Runtime) finishPendingDraft(session core.AISession, message string) {
	session.PendingDraft = nil
	session.Messages = append(session.Messages, core.AISessionMessage{
		ID:        uuid.New().String(),
		Role:      core.AIMessageRoleAssistant,
		Content:   message,
		CreatedAt: time.Now(),
	})
	session.UpdatedAt = time.Now()
	_ = r.Sessions.Save(session)
}
