package core

// UPDATE 阶段配置
type UpdateConfig struct {
	DeltaType    string `json:"delta_type"    toml:"delta_type"` // interval|cron|manual
	DeltaSeconds int64  `json:"delta_seconds" toml:"delta_seconds"`
	CronExpr     string `json:"cron_expr,omitempty" toml:"cron_expr"`
}

// 连线定义（持久化格式）
type EdgeConfig struct {
	From PortRef `json:"from" toml:"from"`
	To   PortRef `json:"to"   toml:"to"`
}

// 端口引用（节点实例ID + 端口ID）
type PortRef struct {
	Node string `json:"node" toml:"node"`
	Port string `json:"port" toml:"port"`
}

// VariableDef 工作流/集合内部变量定义
type VariableDef struct {
	Name    string   `json:"name"     toml:"name"`
	VarType PortType `json:"var_type" toml:"var_type"`
	Default any      `json:"default,omitempty" toml:"default,omitempty"`
}

// WorkflowDef 工作流定义（持久化到 TOML）
// 注意：
//   - 无 UpdateConfig 字段（Delta 配置移到 system_update 节点的 Config 中）
//   - 无 SubWorkflows 字段（子工作流通过 sub_workflow_call 节点表达）
type WorkflowDef struct {
	ID          string         `json:"id"          toml:"id"`
	Name        string         `json:"name"        toml:"name"`
	Description string         `json:"description" toml:"description"`
	Variables   []VariableDef  `json:"variables"   toml:"variables"`
	Nodes       []NodeInstance `json:"nodes"       toml:"nodes"`
	Edges       []EdgeConfig   `json:"edges"       toml:"edges"`
}
