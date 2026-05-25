// 执行记录数据结构
// ExecutionRecord 是一次运行的完整快照
// 节点状态/日志/变量按调用栈树状组织（RootFrame + Children）
// 同一个集合多次被调用时，按 caller node instance ID 索引区分

package core

import "time"

// ExecutionRecord 单次执行的完整数据
// 通过 Wails 绑定返回给前端时整体序列化为 JSON
type ExecutionRecord struct {
	ID         string            `json:"id"`
	WorkflowID string            `json:"workflow_id"`
	Snapshot   ExecutionSnapshot `json:"snapshot"`     // 启动时的不可变快照
	Status     WorkflowStatus    `json:"status"`       // running / success / failed / terminated
	StartedAt  time.Time         `json:"started_at"`
	FinishedAt *time.Time        `json:"finished_at,omitempty"`
	RootFrame  FrameState        `json:"root_frame"`   // 主流 frame + 嵌套调用的 children
	Error      string            `json:"error,omitempty"`
}

// FrameState 单个调用栈帧的状态（树状递归）
// AssembleID 为空表示主流帧，否则为对应集合 ID
// Children 按 caller node instance ID 索引：同一集合被不同调用节点调用时是不同 frame
type FrameState struct {
	AssembleID string                  `json:"assemble_id"`
	NodeStates map[string]NodeState    `json:"node_states"`
	NodeLogs   map[string][]LogEntry   `json:"node_logs"`
	Variables  map[string]any          `json:"variables"`
	Params     map[string]any          `json:"params,omitempty"`
	Returns    map[string]any          `json:"returns,omitempty"`
	Children   map[string]*FrameState  `json:"children,omitempty"`
}

// ExecutionSnapshot 启动时打的快照
type ExecutionSnapshot struct {
	Workflow  WorkflowDef            `json:"workflow"`
	Assembles map[string]AssembleDef `json:"assembles"` // key = assemble ID
}

// LogEntry 单条节点日志
type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"` // info / warn / error
	Message string    `json:"message"`
}

// ExecutionSummary 列表展示用的精简结构（剥离 snapshot / logs / frames）
type ExecutionSummary struct {
	ID           string         `json:"id"`
	WorkflowID   string         `json:"workflow_id"`
	WorkflowName string         `json:"workflow_name"`
	Status       WorkflowStatus `json:"status"`
	StartedAt    time.Time      `json:"started_at"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
	Error        string         `json:"error,omitempty"`
}

// Summary 从 ExecutionRecord 派生 ExecutionSummary
func (r ExecutionRecord) Summary() ExecutionSummary {
	return ExecutionSummary{
		ID:           r.ID,
		WorkflowID:   r.WorkflowID,
		WorkflowName: r.Snapshot.Workflow.Name,
		Status:       r.Status,
		StartedAt:    r.StartedAt,
		FinishedAt:   r.FinishedAt,
		Error:        r.Error,
	}
}
