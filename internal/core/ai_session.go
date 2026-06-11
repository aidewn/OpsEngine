// AI 助手多会话数据结构，跨进程持久化到 data/ai-sessions/<id>.toml。

package core

import "time"

// AISessionScope 标识会话的工作范围。
//
//   - AISessionScopeEnvironment：环境级会话，Agent 可看到环境下所有配置，适合架构分析、多机巡检。
//   - AISessionScopeConfig：单配置会话（旧行为），强绑某个 SSH/Docker/K8s 配置。
//   - AISessionScopeGeneral：通用会话，不绑定环境，适合生成/修改工作流、集合等可复用资产。
//
// 新建会话默认环境级；显式指定 ConfigID 时才会落到 config 范围。
// 旧会话文件没有 Scope 字段，store 加载时按 ConfigID 是否为空自动补值（参见 AISessionStore.loadLocked）。
type AISessionScope string

const (
	AISessionScopeGeneral     AISessionScope = "general"
	AISessionScopeEnvironment AISessionScope = "environment"
	AISessionScopeConfig      AISessionScope = "config"
)

// AISessionMessageRole 是会话消息的角色枚举。
type AISessionMessageRole string

const (
	// AIMessageRoleSystem 是注入的系统提示词或预取的服务器上下文，前端不展示但参与 LLM 调用。
	AIMessageRoleSystem AISessionMessageRole = "system"
	// AIMessageRoleUser 是用户输入。
	AIMessageRoleUser AISessionMessageRole = "user"
	// AIMessageRoleAssistant 是模型回复。
	AIMessageRoleAssistant AISessionMessageRole = "assistant"
)

// AISessionMessage 是一条会话消息。
type AISessionMessage struct {
	ID           string               `json:"id"            toml:"id"`
	Role         AISessionMessageRole `json:"role"          toml:"role"`
	Content      string               `json:"content"       toml:"content"`
	Progress     []string             `json:"progress,omitempty"      toml:"progress,omitempty"`
	WorkflowID   string               `json:"workflow_id,omitempty"   toml:"workflow_id,omitempty"`
	WorkflowName string               `json:"workflow_name,omitempty" toml:"workflow_name,omitempty"`
	AssembleID   string               `json:"assemble_id,omitempty"   toml:"assemble_id,omitempty"`
	AssembleName string               `json:"assemble_name,omitempty" toml:"assemble_name,omitempty"`
	ArtifactType string               `json:"artifact_type,omitempty" toml:"artifact_type,omitempty"`
	ActionType   string               `json:"action_type,omitempty"   toml:"action_type,omitempty"`
	// DocID / DocTitle 指向消息产生时落地的 OpsDoc，前端用于"查看文档"跳转。
	// 当前仅 architecture handler 在使用；troubleshoot 通过用户手动"保存为报告"产文档，不预填这两字段。
	DocID    string `json:"doc_id,omitempty"    toml:"doc_id,omitempty"`
	DocTitle string `json:"doc_title,omitempty" toml:"doc_title,omitempty"`
	// Hidden 标记 system 角色的辅助消息（如预取的服务器信息），前端不渲染。
	Hidden bool `json:"hidden,omitempty" toml:"hidden,omitempty"`
	// Intent 是 assistant 消息产生时的意图标签（"chat" / "troubleshoot" / "inspect_server" / ...）。
	// 前端据此决定是否显示"保存为报告"按钮，后端 SaveAssistantMessageAsDoc 据此推断 OpsDoc.Kind。
	// 旧消息没有此字段，按空字符串处理。
	Intent    string    `json:"intent,omitempty" toml:"intent,omitempty"`
	// NodeCount 工作流/集合消息附带的节点数量摘要，供前端 ActionCard 展示。
	NodeCount int `json:"node_count,omitempty" toml:"node_count,omitempty"`
	// ChangeSummary 更新类消息的结构变更摘要（如「节点 5→7」）。
	ChangeSummary string `json:"change_summary,omitempty" toml:"change_summary,omitempty"`
	CreatedAt time.Time `json:"created_at" toml:"created_at"`
}

// AISession 是一次完整的 AI 对话上下文。
type AISession struct {
	ID    string `json:"id"    toml:"id"`
	Title string `json:"title" toml:"title"`
	// EnvironmentID 在创建时绑定，必填，会话期间不可变。
	EnvironmentID string `json:"environment_id" toml:"environment_id"`
	// Scope 决定 Agent 看到的资产范围。空字符串视为 config 范围（仅旧会话）。
	Scope AISessionScope `json:"scope" toml:"scope"`
	// ConfigID 仅当 Scope=config 时有意义；环境级会话也可设置为"用户偏好的默认目标"。
	ConfigID string `json:"config_id" toml:"config_id"`
	// ContextPrefetched 标记是否已经预取过 SSH 服务器信息，避免每条消息重复采集。
	ContextPrefetched bool               `json:"context_prefetched" toml:"context_prefetched"`
	// ActiveArtifactType / ActiveArtifactID / ActiveArtifactName 是会话级「编辑模式」上下文。
	// 对标 Claude Code 的 artifact 迭代：用户点「继续修改」后持久化，刷新不丢。
	ActiveArtifactType string `json:"active_artifact_type,omitempty" toml:"active_artifact_type,omitempty"`
	ActiveArtifactID   string `json:"active_artifact_id,omitempty"   toml:"active_artifact_id,omitempty"`
	ActiveArtifactName string `json:"active_artifact_name,omitempty" toml:"active_artifact_name,omitempty"`
	// PendingDraft 是等待用户确认应用的 AI 修改草案（确认模式下 update/fix 路径产生）。
	PendingDraft *AIPendingDraft    `json:"pending_draft,omitempty" toml:"pending_draft,omitempty"`
	Messages     []AISessionMessage `json:"messages"   toml:"messages"`
	CreatedAt    time.Time          `json:"created_at" toml:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at" toml:"updated_at"`
}

// AIPendingDraft 是 AI 产出但尚未落盘的资产修改草案。
// 应用前会校验 BaseHash：基线工作流在生成后被手工修改过则拒绝应用，避免静默覆盖。
type AIPendingDraft struct {
	ArtifactType  string    `json:"artifact_type"  toml:"artifact_type"` // 目前仅 "workflow"
	ArtifactID    string    `json:"artifact_id"    toml:"artifact_id"`
	ArtifactName  string    `json:"artifact_name"  toml:"artifact_name"`
	DraftJSON     string    `json:"draft_json"     toml:"draft_json"` // 落地校验后的完整资产 JSON
	BaseHash      string    `json:"base_hash"      toml:"base_hash"`  // 生成时基线资产的 SHA-256
	ChangeSummary string    `json:"change_summary" toml:"change_summary"`
	CreatedAt     time.Time `json:"created_at"     toml:"created_at"`
}
