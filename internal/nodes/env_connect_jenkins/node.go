// env_connect_jenkins 节点：从环境配置读取 Jenkins 凭证并建立 HTTP 客户端

package env_connect_jenkins

import (
	"fmt"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node env_connect_jenkins 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "env_connect_jenkins",
		DisplayName: "环境 · Jenkins 连接",
		Category:    "environment",
		NodeKind:    core.NodeKindAction,
		Icon:        "🛠",
		Description: "从环境配置读取 Jenkins base_url/user/api_token，Ping 后输出 JenkinsContext 句柄",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "client", Label: "Jenkins", PortType: core.PortTypeJenkinsContext},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "env_select", ID: "environment_id", Label: "环境", Required: true},
			{Type: "env_config_select", ID: "config_id", Label: "Jenkins 配置",
				Required: true, ConfigKindFilter: string(core.EnvConfigKindJenkins)},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 解析环境 → 找 jenkins 配置 → 构造客户端 → Ping
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	envID := strings.TrimSpace(ctx.ConfigString("environment_id"))
	configID := strings.TrimSpace(ctx.ConfigString("config_id"))
	if envID == "" {
		return nil, fmt.Errorf("environment_id 未配置")
	}
	if configID == "" {
		return nil, fmt.Errorf("config_id 未配置")
	}
	envStore := ctx.EnvironmentStore()
	if envStore == nil {
		return nil, fmt.Errorf("引擎未配置 environmentStore")
	}
	env, err := envStore.Get(envID)
	if err != nil {
		return nil, err
	}
	item, err := findJenkinsConfig(env, configID)
	if err != nil {
		return nil, err
	}

	baseURL := strings.TrimSpace(stringField(item.Fields, "base_url"))
	user := strings.TrimSpace(stringField(item.Fields, "user"))
	token := stringField(item.Fields, "api_token")
	timeout := intField(item.Fields, "timeout_seconds", 10)

	ctx.Info("连接 Jenkins %s (user=%s)", baseURL, user)
	client, err := clients.NewJenkinsClient(baseURL, user, token, timeout)
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx.Context()); err != nil {
		return nil, err
	}
	ctx.Info("Jenkins 连接成功: %s", baseURL)
	return engine.Outputs{
		"client": client,
	}, nil
}

// findJenkinsConfig 在环境中按 configID 定位 jenkins 配置
func findJenkinsConfig(env core.EnvironmentDef, configID string) (*core.EnvConfigItem, error) {
	for i := range env.Configs {
		c := &env.Configs[i]
		if c.ID == configID {
			if c.Kind != core.EnvConfigKindJenkins {
				return nil, fmt.Errorf("配置 %s 类型 %s 不是 jenkins", configID, c.Kind)
			}
			return c, nil
		}
	}
	return nil, fmt.Errorf("配置未找到: %s", configID)
}

// ── 取值辅助 ──────────────────────────────────────────────

func stringField(fields map[string]any, key string) string {
	if fields == nil {
		return ""
	}
	if v, ok := fields[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func intField(fields map[string]any, key string, fallback int) int {
	if fields == nil {
		return fallback
	}
	v, ok := fields[key]
	if !ok {
		return fallback
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return fallback
}
