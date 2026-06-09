// Runtime 的外部依赖接口。
// 通过接口而不是具体类型注入，使得：
//   - 业务路径（ai.go）传入真实 store / Wails emitter
//   - 测试路径传入内存 store / 缓冲 emitter / stub LLM
//
// 所有接口都保持最小：只暴露 Runtime 真正用到的方法，避免泄露存储层细节。

package runtime

import (
	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
)

// SessionStore 仅暴露 Runtime 需要的 Get / Save。List/Delete 等仍由 Wails 入口直接调用 store。
type SessionStore interface {
	Get(id string) (core.AISession, error)
	Save(session core.AISession) error
}

// WorkflowSaver 让 Runtime 只能写工作流，不能列举或删除，缩小可影响范围。
type WorkflowSaver interface {
	Save(wf core.WorkflowDef) error
}

// OpsDocSaver 让 Runtime 只能写文档；列举/删除走 ai.go 的 RPC。
type OpsDocSaver interface {
	Save(doc core.OpsDoc) error
}

// EnvironmentLookup 按 id 取环境定义，用于 SSH 上下文预取与校验。
type EnvironmentLookup func(environmentID string) (core.EnvironmentDef, error)

// EnvironmentLister 列出全部环境，仅用于工作流生成 prompt 时的目录注入。
type EnvironmentLister func() ([]core.EnvironmentDef, error)

// NodeCatalog 返回当前可用节点类型，用于工作流生成 prompt。
type NodeCatalog func() []core.NodeTypeDef

// NodeTypeChecker 判断 type_id 是否对应已注册节点或现存集合。
type NodeTypeChecker func(typeID string) error

// LLMProvider 是模型调用的最小接口。
// 把超时上下文与连接细节封装在实现侧（ai.go 的 llmAdapter），让 Runtime 不感知 base_url / api_key。
type LLMProvider interface {
	Chat(messages []clients.ChatMessage) (string, error)
	ChatStream(messages []clients.ChatMessage, onDelta func(string)) (string, error)
	// ChatWithTools 支持 OpenAI function calling 协议；非流式。
	// 工具未启用时（tools 为 nil 或空），等价于 Chat 但返回 ChatCompletion 结构。
	ChatWithTools(messages []clients.ChatMessage, tools []clients.ToolSpec) (clients.ChatCompletion, error)
}
