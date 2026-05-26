package core

// EnvironmentDef 业务/项目环境
type EnvironmentDef struct {
	ID          string          `json:"id"          toml:"id"`
	Name        string          `json:"name"        toml:"name"`
	Description string          `json:"description" toml:"description"`
	Configs     []EnvConfigItem `json:"configs"     toml:"configs"`
}

// EnvConfigKind 配置类型
type EnvConfigKind string

const (
	EnvConfigKindSSH       EnvConfigKind = "ssh"
	EnvConfigKindDocker    EnvConfigKind = "docker"
	EnvConfigKindK8s       EnvConfigKind = "k8s"
	EnvConfigKindJenkins   EnvConfigKind = "jenkins"
	EnvConfigKindLocalhost EnvConfigKind = "localhost"
)

// EnvConfigItem 环境内单条配置（fields 按 kind 解析）
type EnvConfigItem struct {
	ID          string         `json:"id"          toml:"id"`
	Name        string         `json:"name"        toml:"name"`
	Kind        EnvConfigKind  `json:"kind"        toml:"kind"`
	Description string         `json:"description" toml:"description"`
	Fields      map[string]any `json:"fields"      toml:"fields"`
}
