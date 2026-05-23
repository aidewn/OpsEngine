package core

import "github.com/google/uuid"

// UPDATE 阶段配置
type UpdateConfig struct {
	DeltaType    string `json:"delta_type"    toml:"delta_type"` // interval|cron|manual
	DeltaSeconds int64  `json:"delta_seconds" toml:"delta_seconds"`
	CronExpr     string `json:"cron_expr,omitempty" toml:"cron_expr"`
}

// 连线定义
type Edge struct {
	EdgeID       uuid.UUID `json:"edge_id"`
	SourceNodeID uuid.UUID `json:"source_node_id"`
	SourcePortID string    `json:"source_port_id"`
	TargetNodeID uuid.UUID `json:"target_node_id"`
	TargetPortID string    `json:"target_port_id"`
}

// TOML 配置中连线的简化格式
type EdgeConfig struct {
	From PortRef `toml:"from"`
	To   PortRef `toml:"to"`
}

type PortRef struct {
	Node string `toml:"node"`
	Port string `toml:"port"`
}

// 工作流定义（持久化到 TOML）
// 注意：
//   - 无 UpdateConfig 字段（Delta 配置移到 system_update 节点的 Config 中）
//   - 无 SubWorkflows 字段（子工作流通过 sub_workflow_call 节点表达）
type WorkflowDef struct {
	ID          string         `json:"id"          toml:"id"`
	Name        string         `json:"name"        toml:"name"`
	Description string         `json:"description" toml:"description"`
	Nodes       []NodeInstance `json:"nodes"       toml:"nodes"`
	Edges       []EdgeConfig   `json:"edges"       toml:"edges"`
}
