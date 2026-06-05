// 把节点目录和环境列表压缩成 prompt 友好的精简结构。
// 目的：模型只看到 id / 类型 / 端口 / 必填项，避免泄露任何敏感字段（如 SSH 密码、Token）。

package prompt

import "OpsEngine/internal/core"

// NodeTypeSummary 是注入 prompt 的精简节点描述。
type NodeTypeSummary struct {
	TypeID       string          `json:"type_id"`
	Name         string          `json:"name"`
	Kind         core.NodeKind   `json:"kind"`
	Description  string          `json:"description,omitempty"`
	InputPorts   []PortSummary   `json:"in,omitempty"`
	OutputPorts  []PortSummary   `json:"out,omitempty"`
	ConfigSchema []SchemaSummary `json:"config,omitempty"`
}

// PortSummary 是端口的精简描述。
type PortSummary struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// SchemaSummary 是配置字段的精简描述。
type SchemaSummary struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
	Default  any    `json:"default,omitempty"`
}

// EnvSummary 是注入 prompt 的环境摘要（不带任何敏感字段）。
type EnvSummary struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	Configs []EnvConfigSummary `json:"configs,omitempty"`
}

// EnvConfigSummary 是单条环境配置的精简描述。
type EnvConfigSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// SummarizeNodeTypes 把完整节点定义压缩成 prompt 友好格式。
func SummarizeNodeTypes(defs []core.NodeTypeDef) []NodeTypeSummary {
	out := make([]NodeTypeSummary, 0, len(defs))
	for _, d := range defs {
		out = append(out, NodeTypeSummary{
			TypeID:       d.TypeID,
			Name:         d.DisplayName,
			Kind:         d.NodeKind,
			Description:  d.Description,
			InputPorts:   summarizePorts(d.InputPorts),
			OutputPorts:  summarizePorts(d.OutputPorts),
			ConfigSchema: summarizeSchema(d.ConfigSchema),
		})
	}
	return out
}

// SummarizeEnvironments 把环境列表脱敏后注入 prompt（只保留 id/name/kind）。
func SummarizeEnvironments(envs []core.EnvironmentDef) []EnvSummary {
	out := make([]EnvSummary, 0, len(envs))
	for _, env := range envs {
		configs := make([]EnvConfigSummary, 0, len(env.Configs))
		for _, c := range env.Configs {
			configs = append(configs, EnvConfigSummary{ID: c.ID, Name: c.Name, Kind: string(c.Kind)})
		}
		out = append(out, EnvSummary{ID: env.ID, Name: env.Name, Configs: configs})
	}
	return out
}

// summarizePorts 抽取端口的 id 与类型。
func summarizePorts(ports []core.PortDef) []PortSummary {
	if len(ports) == 0 {
		return nil
	}
	out := make([]PortSummary, 0, len(ports))
	for _, p := range ports {
		out = append(out, PortSummary{ID: p.ID, Type: string(p.PortType)})
	}
	return out
}

// summarizeSchema 抽取配置字段的 id/type/required/default。
func summarizeSchema(fields []core.FieldSchema) []SchemaSummary {
	if len(fields) == 0 {
		return nil
	}
	out := make([]SchemaSummary, 0, len(fields))
	for _, f := range fields {
		out = append(out, SchemaSummary{
			ID:       f.ID,
			Type:     f.Type,
			Required: f.Required,
			Default:  f.Default,
		})
	}
	return out
}
