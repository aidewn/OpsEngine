// 普通运维问答的单轮处理。
//
// 关键流程：
//   - 注入环境资产清单（Inventory），让 Agent 看到环境全貌而不是只盯单 SSH
//   - 若会话有明确 SSH 目标（Scope=config 或 Inventory 内唯一 SSH），刷新过期的 SSH 快照
//   - 长会话裁剪，控制上下文窗口
//   - 流式回复

package runtime

import (
	"strings"
	"time"

	agentctx "OpsEngine/internal/agent/context"
	"OpsEngine/internal/agent/prompt"
	"OpsEngine/internal/core"

	"github.com/google/uuid"
)

// handleChat 处理 intent.KindChat。
func (r *Runtime) handleChat(req Request, session core.AISession) {
	progress := []string{}

	// 构造 Inventory：注入到 prompt 让 LLM 知道环境内有哪些资产。
	// 失败不致命，仅打印 progress 提示，继续走 LLM。
	inv, inventoryText := r.buildInventory(req, session, &progress)

	// 若存在可用 SSH 目标，按 TTL 刷新快照。
	if target := chooseSSHTarget(inv, session); target != "" {
		r.refreshSnapshotIfStale(req, &session, target, &progress)
	}

	maxTurns := r.MaxTurns
	if maxTurns <= 0 {
		maxTurns = agentctx.DefaultMaxTurns
	}
	trimmed := agentctx.TruncateMessages(session.Messages, maxTurns)
	messages, err := prompt.BuildChatMessages(trimmed, prompt.ChatContext{Inventory: inventoryText})
	if err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	assistant := strings.Builder{}
	_, err = r.LLM.ChatStream(messages, func(delta string) {
		assistant.WriteString(delta)
		r.Emit.Emit(Event{
			RequestID: req.RequestID, SessionID: session.ID,
			Type: EventDelta, Text: delta,
		})
	})
	if err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	session.Messages = append(session.Messages, core.AISessionMessage{
		ID:        uuid.New().String(),
		Role:      core.AIMessageRoleAssistant,
		Content:   assistant.String(),
		Progress:  progress,
		CreatedAt: time.Now(),
	})
	session.UpdatedAt = time.Now()
	if err := r.Sessions.Save(session); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}
	r.emitDone(req.RequestID, session.ID)
}

// buildInventory 加载环境定义并构造资产清单。
// 返回 Inventory 与渲染好的文本；任意一步失败时返回零值，调用方继续往下走。
func (r *Runtime) buildInventory(req Request, session core.AISession, progress *[]string) (agentctx.Inventory, string) {
	if r.Environments == nil {
		return agentctx.Inventory{}, ""
	}
	env, err := r.Environments(session.EnvironmentID)
	if err != nil {
		r.emitProgress(req.RequestID, session.ID, "环境信息加载失败："+err.Error(), progress)
		return agentctx.Inventory{}, ""
	}
	inv := agentctx.BuildInventory(env)
	return inv, inv.RenderText()
}

// chooseSSHTarget 决定 chat 流程要不要拉 SSH 快照、拉哪台。
//   - Scope=config 且 ConfigID 已设：用 ConfigID（旧行为）
//   - Scope=environment：若 Inventory 中只有一台 SSH，用它；否则跳过快照
//
// 让 Agent 在多 SSH 环境下不会"擅自选一台"，把目标决定权交给用户后续追问或巡检流程。
func chooseSSHTarget(inv agentctx.Inventory, session core.AISession) string {
	if strings.TrimSpace(session.ConfigID) != "" {
		return session.ConfigID
	}
	if inv.IsEmpty() {
		return ""
	}
	return inv.SingleSSH()
}

// refreshSnapshotIfStale 检查 SSH 快照是否新鲜，过期则丢旧拉新。
// 采集失败不致命，继续走 LLM，仅在 progress 中告知用户。
func (r *Runtime) refreshSnapshotIfStale(req Request, session *core.AISession, configID string, progress *[]string) {
	ttl := r.SnapshotTTL
	if ttl <= 0 {
		ttl = agentctx.DefaultSnapshotTTL
	}
	if agentctx.SnapshotFresh(*session, ttl) {
		return
	}
	hint := "正在采集服务器信息"
	if !agentctx.LatestSnapshotAt(*session).IsZero() {
		hint = "服务器信息已过期，正在重新采集"
	}
	r.emitProgress(req.RequestID, session.ID, hint, progress)
	text, err := agentctx.Prefetch(agentctx.EnvironmentLookup(r.Environments), session.EnvironmentID, configID, agentctx.DefaultPrefetchTimeout)
	if err != nil {
		notice := "服务器信息采集失败：" + err.Error()
		r.emitProgress(req.RequestID, session.ID, notice, progress)
		return
	}
	session.Messages = agentctx.PruneSnapshots(session.Messages)
	session.Messages = append(session.Messages, core.AISessionMessage{
		ID:        uuid.New().String(),
		Role:      core.AIMessageRoleSystem,
		Content:   text,
		Hidden:    true,
		CreatedAt: time.Now(),
	})
	session.ContextPrefetched = true
	if err := r.Sessions.Save(*session); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
	}
}
