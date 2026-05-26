// env_connect_ssh 节点：从环境配置中读取 SSH 凭证并建立连接
// 与 ssh_with_linux 的区别：凭证来自环境（可复用）而非节点 config 内联

package env_connect_ssh

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

const (
	defaultPort           = 22
	defaultTimeoutSeconds = 10
)

func init() { engine.Register(&Node{}) }

// Node env_connect_ssh 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "env_connect_ssh",
		DisplayName: "环境 · SSH 连接",
		Category:    "environment",
		NodeKind:    core.NodeKindAction,
		Icon:        "🔌",
		Description: "从环境配置读取 SSH 凭证并建立连接，输出 LinuxSshConnection 句柄",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "client", Label: "SSH", PortType: core.PortTypeLinuxSsh},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "env_select", ID: "environment_id", Label: "环境", Required: true},
			{Type: "env_config_select", ID: "config_id", Label: "SSH 配置",
				Required: true, ConfigKindFilter: string(core.EnvConfigKindSSH)},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 解析环境 → 找 ssh 配置 → DialLinuxSsh → 输出句柄
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
		return nil, fmt.Errorf("引擎未配置 environmentStore，无法解析环境")
	}
	env, err := envStore.Get(envID)
	if err != nil {
		return nil, err
	}

	item, err := findSSHConfig(env, configID)
	if err != nil {
		return nil, err
	}

	host := strings.TrimSpace(stringField(item.Fields, "host"))
	user := strings.TrimSpace(stringField(item.Fields, "user"))
	password := stringField(item.Fields, "password")
	port := intField(item.Fields, "port", defaultPort)
	timeout := intField(item.Fields, "timeout_seconds", defaultTimeoutSeconds)
	if host == "" {
		return nil, fmt.Errorf("SSH 配置缺少 host")
	}
	if user == "" {
		return nil, fmt.Errorf("SSH 配置缺少 user")
	}
	if password == "" {
		return nil, fmt.Errorf("SSH 配置缺少 password")
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	ctx.Info("正在连接环境 %s 内 SSH %s@%s", env.Name, user, addr)

	client, err := clients.DialLinuxSsh(host, port, user, password, timeout)
	if err != nil {
		return nil, err
	}
	ctx.Info("SSH 连接成功: %s@%s", user, addr)
	return engine.Outputs{
		"client": client,
	}, nil
}

// findSSHConfig 按 configID 在环境中定位 ssh 配置；非 ssh 或不存在均报错
func findSSHConfig(env core.EnvironmentDef, configID string) (*core.EnvConfigItem, error) {
	for i := range env.Configs {
		c := &env.Configs[i]
		if c.ID == configID {
			if c.Kind != core.EnvConfigKindSSH {
				return nil, fmt.Errorf("配置 %s 类型 %s 不是 ssh", configID, c.Kind)
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
