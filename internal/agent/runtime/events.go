// Runtime 输出事件协议。
// 设计目标：让 Runtime 不直接依赖 Wails，前端适配器（ai.go 里的 wailsEmitter）只做协议转译。
// 后续 P7 工具调用、报告生成等都通过新增 Type 扩展，不破坏现有事件契约。

package runtime

// EventType 是事件分类的字符串枚举。前端 ai 助手对话框按 Type 分流渲染。
type EventType string

const (
	// EventProgress 用于阶段性进度文本（"正在解析模型返回"）。
	EventProgress EventType = "progress"
	// EventDelta 流式回复的增量片段。
	EventDelta EventType = "delta"
	// EventError 错误终态，调用方不应在此后再 Emit。
	EventError EventType = "error"
	// EventDone 成功终态。
	EventDone EventType = "done"
	// EventWorkflow 工作流生成完成的副带数据（workflow id/name）。
	EventWorkflow EventType = "workflow"
	// EventTargetSelect 表示本轮需要用户选择目标配置后才能继续。
	EventTargetSelect EventType = "target_select"
)

// TargetOption 是需要用户选择的目标配置候选项。
type TargetOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Event 是一条对外推送的事件。字段保持扁平方便序列化。
type Event struct {
	RequestID     string         `json:"request_id"`
	SessionID     string         `json:"session_id,omitempty"`
	Type          EventType      `json:"type"`
	Text          string         `json:"text,omitempty"`
	WorkflowID    string         `json:"workflow_id,omitempty"`
	WorkflowName  string         `json:"workflow_name,omitempty"`
	TargetOptions []TargetOption `json:"target_options,omitempty"`
}

// Emitter 把 Runtime 事件投递到外部传输层（Wails / WebSocket / 测试用 buffer）。
type Emitter interface {
	Emit(event Event)
}

// EmitterFunc 让普通函数可以作为 Emitter 使用，避免每个调用方都定义结构体。
type EmitterFunc func(event Event)

// Emit 实现 Emitter 接口。
func (f EmitterFunc) Emit(event Event) { f(event) }
