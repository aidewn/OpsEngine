// chat / troubleshoot 单轮处理。两者结构相同（注入 Inventory → 可选 SSH 快照 → 工具循环或流式回复），
// 只在 system 提示词与 Intent 标签上有差别，因此共用 runConversationTurn。
//
// 关键流程：
//   - 注入环境资产清单（Inventory），让 Agent 看到环境全貌而不是只盯单 SSH
//   - 若会话有明确 SSH 目标（Scope=config 或 Inventory 内唯一 SSH），刷新过期的 SSH 快照
//   - 长会话裁剪，控制上下文窗口
//   - 工具循环（启用 Tools 时）或流式回复

package runtime

import (
	"strings"
	"time"

	agentctx "OpsEngine/internal/agent/context"
	"OpsEngine/internal/agent/prompt"
	"OpsEngine/internal/core"

	"github.com/google/uuid"
)

// turnOpts 把 chat / troubleshoot / 工作流生成 之间的差异收敛到一个结构里。
type turnOpts struct {
	// SystemKind 决定加载哪个 system 提示词。
	SystemKind prompt.SystemPromptKind
	// IntentTag 写到 AISessionMessage.Intent，前端据此决定是否显示"保存为报告"。
	IntentTag string
	// ExtraSystem 场景化追加的 system 段（如工作流编排任务引导），空时不注入。
	ExtraSystem string
}

// handleChat 处理 intent.KindChat。
func (r *Runtime) handleChat(req Request, session core.AISession) {
	r.runConversationTurn(req, session, turnOpts{
		SystemKind: prompt.SystemPromptChat,
		IntentTag:  "chat",
	})
}

// handleTroubleshoot 处理 intent.KindTroubleshoot：与 chat 同结构，但 system 提示词强制"事实/判断/建议"。
func (r *Runtime) handleTroubleshoot(req Request, session core.AISession) {
	r.runConversationTurn(req, session, turnOpts{
		SystemKind: prompt.SystemPromptTroubleshoot,
		IntentTag:  "troubleshoot",
	})
}

// runConversationTurn 是 chat 和 troubleshoot 共享的执行体。
func (r *Runtime) runConversationTurn(req Request, session core.AISession, opts turnOpts) {
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
	messages, err := prompt.BuildChatMessages(trimmed, prompt.ChatContext{
		SystemKind: opts.SystemKind,
		Inventory:  inventoryText,
		Extra:      opts.ExtraSystem,
	})
	if err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	assistant := strings.Builder{}
	if r.Tools != nil && !r.Tools.IsEmpty() {
		// 启用工具时走流式工具循环：runChatToolLoop 内部对每轮 LLM 调用
		// 走 ChatWithToolsStream 并把 content delta 直接 Emit，所以这里
		// 拿到的 text 已经被前端渲染过；只需累积进 assistant 用于落库。
		text, err := r.runChatToolLoop(req, &session, messages, &progress)
		if err != nil {
			r.emitTurnError(req, &session, err.Error(), progress, opts.IntentTag)
			return
		}
		assistant.WriteString(text)
	} else {
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
	}

	session.Messages = append(session.Messages, core.AISessionMessage{
		ID:        uuid.New().String(),
		Role:      core.AIMessageRoleAssistant,
		Content:   assistant.String(),
		Progress:  progress,
		Intent:    opts.IntentTag,
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
	if strings.TrimSpace(session.EnvironmentID) == "" {
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
