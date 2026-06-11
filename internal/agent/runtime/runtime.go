// Runtime 是单轮 AI 助手执行的"控制塔"。
//
// 输入：一次用户消息 Request。
// 流程：
//  1. 加载会话与设置（由调用方负责传入）
//  2. 把用户消息追加到 session.Messages
//  3. intent.Resolve 路由到对应 handler
//  4. handler 执行业务（chat 流式 / 工作流生成 / 巡检计划）
//  5. handler 内部直接 Emit 事件 + 写回会话
//
// 设计要点：
//   - Runtime 只协调流程，业务逻辑全部下沉到 handler 文件
//   - handler 之间不共享状态，避免隐式耦合
//   - 任何错误都通过 emitError 推到事件流，Run 自身只返回参数级错误

package runtime

import (
	"errors"
	"strings"
	"time"

	"OpsEngine/internal/agent/intent"
	"OpsEngine/internal/agent/tools"
	"OpsEngine/internal/core"

	"github.com/google/uuid"
)

// Request 是单轮调用的输入。Operation 沿用 ai.go 历史命名，留给前端显式指定意图。
type Request struct {
	RequestID string
	SessionID string
	Operation string
	Message   string
	// TargetConfigID 是前端在 target_select 后回传的本轮目标配置；只影响当前 turn。
	TargetConfigID string
	// ArtifactType / ArtifactID 指向用户正在迭代的资产。
	ArtifactType string
	ArtifactID   string
	// ExecutionID 指向要修复的失败执行，仅 operation=fix_execution 时有效。
	ExecutionID string
}

// Runtime 持有完成一轮所需的全部依赖。
// 字段都是接口或回调，方便测试时替换。
type Runtime struct {
	Sessions     SessionStore
	Workflows    WorkflowSaver
	Assembles    AssembleSaver
	OpsDocs      OpsDocSaver
	Environments EnvironmentLookup
	EnvList      EnvironmentLister
	Nodes        NodeCatalog
	NodeChecker  NodeTypeChecker
	Executions   ExecutionGetter
	LLM          LLMProvider
	Emit         Emitter
	// Tools 是只读 Agent 工具注册表。可为 nil（chat 路径退化为不带工具的单轮调用）。
	Tools *tools.Registry

	// SnapshotTTL 覆盖默认 SSH 快照过期时间。<=0 时用包默认值。
	SnapshotTTL time.Duration
	// MaxTurns 覆盖默认长会话裁剪轮次。<=0 时用包默认值。
	MaxTurns int
	// MaxToolRounds 限制 chat 工具循环最多迭代多少轮，<=0 时用默认 5。
	MaxToolRounds int
	// ApplyMode 控制 AI 修改已有工作流的落盘策略：
	// "confirm" 先暂存草案待用户确认；其余值（含空）直接保存。
	ApplyMode string
}

// Run 执行一轮 AI 助手调用。
// 参数级错误（必填字段缺失、会话不存在）通过返回值上报；
// 业务级错误（LLM 失败、解析失败）通过 EventError 推到前端，本函数返回 nil。
func (r *Runtime) Run(req Request) error {
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.Message = strings.TrimSpace(req.Message)
	req.TargetConfigID = strings.TrimSpace(req.TargetConfigID)
	req.ArtifactType = strings.TrimSpace(req.ArtifactType)
	req.ArtifactID = strings.TrimSpace(req.ArtifactID)
	req.ExecutionID = strings.TrimSpace(req.ExecutionID)

	if req.RequestID == "" {
		return errors.New("request_id 不能为空")
	}
	if req.SessionID == "" {
		r.emitError(req.RequestID, "", "session_id 不能为空")
		return nil
	}
	if req.Message == "" {
		r.emitError(req.RequestID, req.SessionID, "请输入要发送给 AI 的内容")
		return nil
	}
	if r.Sessions == nil {
		r.emitError(req.RequestID, req.SessionID, "会话存储未初始化")
		return nil
	}

	session, err := r.Sessions.Get(req.SessionID)
	if err != nil {
		r.emitError(req.RequestID, req.SessionID, err.Error())
		return nil
	}

	// 追加用户消息并在首条时自动生成标题。
	// 若本轮是 target_select 后的继续执行，前端会带回相同 Message 和 TargetConfigID，
	// 此时上一条 user 消息已保存，不能重复写入会话历史。
	now := time.Now()
	if !shouldReuseLastUserMessage(session, req) {
		session.Messages = append(session.Messages, core.AISessionMessage{
			ID:        uuid.New().String(),
			Role:      core.AIMessageRoleUser,
			Content:   req.Message,
			CreatedAt: now,
		})
	}
	if isFirstUserMessage(session) {
		session.Title = makeSessionTitle(req.Message)
	}
	session.UpdatedAt = now
	if err := r.Sessions.Save(session); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return nil
	}

	decision := intent.Resolve(req.Operation, req.Message)
	applyArtifactRouting(&req, session, &decision)
	if isExplicitNewAsset(req.Message) && session.ActiveArtifactID != "" {
		if err := r.clearSessionActiveArtifact(&session); err != nil {
			r.emitError(req.RequestID, session.ID, err.Error())
			return nil
		}
	}

	switch decision.Kind {
	case intent.KindInspectServer:
		r.handleInspection(req, session)
	case intent.KindCreateAssemble:
		r.handleAssemble(req, session, false)
	case intent.KindUpdateAssemble:
		r.handleAssemble(req, session, true)
	case intent.KindUpdateWorkflow:
		r.handleWorkflowUpdate(req, session)
	case intent.KindFixExecution:
		r.handleExecutionFix(req, session)
	case intent.KindGenerateWorkflow:
		r.handleWorkflow(req, session)
	case intent.KindTroubleshoot:
		r.handleTroubleshoot(req, session)
	case intent.KindAnalyzeArchitecture:
		r.handleArchitecture(req, session)
	default:
		r.handleChat(req, session)
	}
	return nil
}

// emitError 是错误终态的便捷封装。
func (r *Runtime) emitError(requestID, sessionID, text string) {
	if r.Emit == nil {
		return
	}
	r.Emit.Emit(Event{RequestID: requestID, SessionID: sessionID, Type: EventError, Text: text})
}

// emitTurnError 将本轮失败写入会话历史后再推送 error 事件，避免错误只留在前端 pending 状态。
func (r *Runtime) emitTurnError(req Request, session *core.AISession, text string, progress []string, intent string) {
	if session != nil && r.Sessions != nil {
		session.Messages = append(session.Messages, core.AISessionMessage{
			ID:        uuid.New().String(),
			Role:      core.AIMessageRoleAssistant,
			Content:   text,
			Progress:  progress,
			Intent:    intent,
			CreatedAt: time.Now(),
		})
		session.UpdatedAt = time.Now()
		_ = r.Sessions.Save(*session)
	}
	r.emitError(req.RequestID, session.ID, text)
}

// emitProgress 是进度事件的便捷封装，同时把文本追加到调用方的 progress 切片。
// 让 handler 调用方可以同时把进度文本写到 session.Messages.Progress 字段，前端展示时序一致。
func (r *Runtime) emitProgress(requestID, sessionID, text string, progress *[]string) {
	if progress != nil {
		*progress = append(*progress, text)
	}
	if r.Emit == nil {
		return
	}
	r.Emit.Emit(Event{RequestID: requestID, SessionID: sessionID, Type: EventProgress, Text: text})
}

// emitHeartbeat 推送瞬态心跳。不写入 progress 切片，前端单行原地刷新。
func (r *Runtime) emitHeartbeat(requestID, sessionID, text string) {
	if r.Emit == nil {
		return
	}
	r.Emit.Emit(Event{RequestID: requestID, SessionID: sessionID, Type: EventHeartbeat, Text: text})
}

// emitDone 推送成功终态。
func (r *Runtime) emitDone(requestID, sessionID string) {
	if r.Emit == nil {
		return
	}
	r.Emit.Emit(Event{RequestID: requestID, SessionID: sessionID, Type: EventDone})
}

// isFirstUserMessage 判断 session.Messages 中是否仅有一条 user 消息。
func isFirstUserMessage(session core.AISession) bool {
	count := 0
	for _, m := range session.Messages {
		if m.Role == core.AIMessageRoleUser {
			count++
			if count > 1 {
				return false
			}
		}
	}
	return count == 1
}

// shouldReuseLastUserMessage 判断 target_select 继续执行时是否复用上一条 user 消息。
func shouldReuseLastUserMessage(session core.AISession, req Request) bool {
	if req.TargetConfigID == "" || len(session.Messages) == 0 {
		return false
	}
	for i := len(session.Messages) - 1; i >= 0; i-- {
		m := session.Messages[i]
		if m.Role == core.AIMessageRoleSystem && m.Hidden {
			continue
		}
		return m.Role == core.AIMessageRoleUser && strings.TrimSpace(m.Content) == req.Message
	}
	return false
}

// makeSessionTitle 从用户第一条消息截取标题，限制 30 个字符。
func makeSessionTitle(message string) string {
	message = strings.TrimSpace(message)
	runes := []rune(message)
	if len(runes) > 30 {
		return string(runes[:30]) + "…"
	}
	if len(runes) == 0 {
		return "新会话"
	}
	return message
}
