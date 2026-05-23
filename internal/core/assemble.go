package core

// AssembleDef 集合定义（用户创建的可复用节点图）
// 集合不能单独运行，只能被工作流或其他集合引用
type AssembleDef struct {
	ID          string         `json:"id"          toml:"id"`
	Name        string         `json:"name"        toml:"name"`
	Description string         `json:"description" toml:"description"`
	Params      []ParamDef     `json:"params"      toml:"params"`
	Returns     []ParamDef     `json:"returns"     toml:"returns"`
	Variables   []VariableDef  `json:"variables"   toml:"variables"`
	Nodes       []NodeInstance `json:"nodes"       toml:"nodes"`
	Edges       []EdgeConfig   `json:"edges"       toml:"edges"`
}

// ParamDef 集合的参数/返回值定义
type ParamDef struct {
	Name    string   `json:"name"     toml:"name"`
	VarType PortType `json:"var_type" toml:"var_type"`
}
