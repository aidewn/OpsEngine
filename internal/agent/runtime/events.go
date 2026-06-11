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
	// EventAssemble 集合生成/更新完成的副带数据（assemble id/name）。
	EventAssemble EventType = "assemble"
	// EventDoc OpsDoc 生成完成的副带数据（doc id/title）。
	// 当前架构分析路径在调用 OpsDocStore.Save 后用它告知前端"查看文档"按钮可用。
	EventDoc EventType = "doc"
	// EventTargetSelect 表示本轮需要用户选择目标配置后才能继续。
	EventTargetSelect EventType = "target_select"
	// EventHeartbeat 表示 Agent 仍在工作的瞬态提示（如"思考中（4s）"）。
	// 与 EventProgress 不同：不写入 session.Messages.Progress，前端用单行原地刷新展示。
	// 出现原因：DeepSeek 在带 tools 请求里通常不真正逐 chunk 推 content，
	// 思考阶段会有 10-60s 的"假死"窗口，需要心跳让用户知道还活着。
	EventHeartbeat EventType = "heartbeat"
	// EventArtifactMode 通知前端进入/退出 artifact 编辑模式。
	EventArtifactMode EventType = "artifact_mode"
	// EventWorkflowPending 确认模式下产生了等待用户应用的修改草案（diff 摘要在 ChangeSummary）。
	EventWorkflowPending EventType = "workflow_pending"
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
	AssembleID    string         `json:"assemble_id,omitempty"`
	AssembleName  string         `json:"assemble_name,omitempty"`
	ArtifactType  string         `json:"artifact_type,omitempty"`
	ActionType    string         `json:"action_type,omitempty"`
	DocID         string         `json:"doc_id,omitempty"`
	DocTitle      string         `json:"doc_title,omitempty"`
	NodeCount     int            `json:"node_count,omitempty"`
	ChangeSummary string         `json:"change_summary,omitempty"`
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
