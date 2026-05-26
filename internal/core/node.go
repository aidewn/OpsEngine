package core

// NodeTypeDef 节点类型定义（注册时确定，驱动前端渲染）
type NodeTypeDef struct {
	TypeID        string        `json:"type_id"`
	DisplayName   string        `json:"display_name"`
	Category      string        `json:"category"`
	NodeKind      NodeKind      `json:"node_kind"` // event|action|pure|flow_control
	Icon          string        `json:"icon"`
	Description   string        `json:"description"`
	InputPorts    []PortDef     `json:"input_ports"`
	OutputPorts   []PortDef     `json:"output_ports"`
	ConfigSchema  []FieldSchema `json:"config_schema"`
	ExecutionMode ExecutionMode `json:"execution_mode"`
}

// PortDef 端口定义
type PortDef struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	PortType PortType `json:"port_type"`
	Required bool     `json:"required"`
}

// FieldSchema 配置字段 Schema（驱动前端表单渲染）
// Type 取值：text | password | number | select | toggle | textarea
//            variable_select | param_select | return_select
//            env_select       —— 下拉所有环境
//            env_config_select —— 依赖同表单 environment_id 字段 + ConfigKindFilter 过滤
type FieldSchema struct {
	Type        string   `json:"type"`
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Placeholder string   `json:"placeholder,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Min         *int64   `json:"min,omitempty"`
	Max         *int64   `json:"max,omitempty"`
	Default     any      `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"`
	// ConfigKindFilter 仅 env_config_select 使用：限定下拉项的 EnvConfigKind
	ConfigKindFilter string `json:"config_kind_filter,omitempty"`
}

// NodeInstance 节点实例（工作流配置中的具体节点）
// 注意：无 Stage 字段，生命周期阶段由可达性分析导出
// InstanceID 使用 string 而非 uuid.UUID，避免 Wails 绑定生成 number[]
type NodeInstance struct {
	InstanceID string         `json:"instance_id" toml:"id"`
	TypeID     string         `json:"type_id"     toml:"type"`
	Config     map[string]any `json:"config"      toml:"config"`
	State      NodeState      `json:"state"       toml:"-"`
	ErrorMsg   string         `json:"error_msg,omitempty" toml:"-"`
	Position   Position       `json:"position"    toml:"position"`
}

type Position struct {
	X float64 `json:"x" toml:"x"`
	Y float64 `json:"y" toml:"y"`
}
