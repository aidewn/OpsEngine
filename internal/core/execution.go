// 执行记录数据结构
// ExecutionRecord 是一次运行的完整快照（含初始 snapshot + 终态）
// ExecutionSummary 是列表用的精简结构

package core

import "time"

// ExecutionRecord 单次执行的完整数据
// 通过 Wails 绑定返回给前端时整体序列化为 JSON
type ExecutionRecord struct {
	ID         string                 `json:"id"`
	WorkflowID string                 `json:"workflow_id"`
	Snapshot   ExecutionSnapshot      `json:"snapshot"`     // 启动时的不可变快照
	Status     WorkflowStatus         `json:"status"`       // running / success / failed / terminated
	StartedAt  time.Time              `json:"started_at"`
	FinishedAt *time.Time             `json:"finished_at,omitempty"`
	NodeStates map[string]NodeState   `json:"node_states"`  // key = instance_id
	NodeLogs   map[string][]LogEntry  `json:"node_logs"`
	Variables  map[string]any         `json:"variables"`    // 主 frame 的变量当前/终态值
	Error      string                 `json:"error,omitempty"`
}

// ExecutionSnapshot 启动时打的快照
// 包含工作流定义以及递归引用到的所有集合定义
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

// ExecutionSummary 列表展示用的精简结构（剥离 snapshot / logs）
type ExecutionSummary struct {
	ID           string         `json:"id"`
	WorkflowID   string         `json:"workflow_id"`
	WorkflowName string         `json:"workflow_name"` // 便利字段，来自 snapshot
	Status       WorkflowStatus `json:"status"`
	StartedAt    time.Time      `json:"started_at"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
	Error        string         `json:"error,omitempty"`
}

// Summary 从 ExecutionRecord 派生 ExecutionSummary
// 用于持久化记录在列表展示时的精简
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
