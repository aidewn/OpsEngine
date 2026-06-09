// AI 助手多会话数据结构，跨进程持久化到 data/ai-sessions/<id>.toml。

package core

import "time"

// AISessionScope 标识会话的工作范围。
//
//   - AISessionScopeEnvironment：环境级会话，Agent 可看到环境下所有配置，适合架构分析、多机巡检。
//   - AISessionScopeConfig：单配置会话（旧行为），强绑某个 SSH/Docker/K8s 配置。
//
// 新建会话默认环境级；显式指定 ConfigID 时才会落到 config 范围。
// 旧会话文件没有 Scope 字段，store 加载时按 ConfigID 是否为空自动补值（参见 AISessionStore.loadLocked）。
type AISessionScope string

const (
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
	Messages          []AISessionMessage `json:"messages"           toml:"messages"`
	CreatedAt         time.Time          `json:"created_at"         toml:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"         toml:"updated_at"`
}
