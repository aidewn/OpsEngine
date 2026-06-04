// AI 助手多会话数据结构，跨进程持久化到 data/ai-sessions/<id>.toml。

package core

import "time"

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
	// Hidden 标记 system 角色的辅助消息（如预取的服务器信息），前端不渲染。
	Hidden    bool      `json:"hidden,omitempty" toml:"hidden,omitempty"`
	CreatedAt time.Time `json:"created_at" toml:"created_at"`
}

// AISession 是一次完整的 AI 对话上下文。
type AISession struct {
	ID    string `json:"id"    toml:"id"`
	Title string `json:"title" toml:"title"`
	// EnvironmentID / ConfigID 在创建时绑定，会话期间不可变。
	EnvironmentID string `json:"environment_id" toml:"environment_id"`
	ConfigID      string `json:"config_id"      toml:"config_id"`
	// ContextPrefetched 标记是否已经预取过 SSH 服务器信息，避免每条消息重复采集。
	ContextPrefetched bool               `json:"context_prefetched" toml:"context_prefetched"`
	Messages          []AISessionMessage `json:"messages"           toml:"messages"`
	CreatedAt         time.Time          `json:"created_at"         toml:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"         toml:"updated_at"`
}
