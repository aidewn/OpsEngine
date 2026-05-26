// env_connect_localhost 节点：在工作流中显式声明「本机执行上下文」
// 与 env_connect_ssh 形态对齐，但底层不拨号：localhost 配置 fields 为空，
// 仅承载「环境归属」语义，方便下游节点统一通过 LocalShellConnection 端口消费

package env_connect_localhost

import (
	"fmt"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node env_connect_localhost 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "env_connect_localhost",
		DisplayName: "环境 · 本机连接",
		Category:    "environment",
		NodeKind:    core.NodeKindAction,
		Icon:        "💻",
		Description: "在工作流中挂载本机 localhost 配置，输出 LocalShellConnection 句柄供下游本地执行节点复用",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "client", Label: "Local", PortType: core.PortTypeLocalShell},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "env_select", ID: "environment_id", Label: "环境", Required: true},
			{Type: "env_config_select", ID: "config_id", Label: "本机配置",
				Required: true, ConfigKindFilter: string(core.EnvConfigKindLocalhost)},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute 解析环境 → 找 localhost 配置 → 直接返回本机句柄
// localhost 不做任何拨号，仅校验配置存在且 kind 正确，与 SSH/Docker 等保持一致的错误风格
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

	if err := findLocalhostConfig(env, configID); err != nil {
		return nil, err
	}

	client := clients.NewLocalShellClient()
	ctx.Info("已挂载本机环境 %s（用户 %s）", env.Name, client.User)
	return engine.Outputs{
		"client": client,
	}, nil
}

// findLocalhostConfig 按 configID 在环境中定位 localhost 配置；非 localhost 或不存在均报错
func findLocalhostConfig(env core.EnvironmentDef, configID string) error {
	for i := range env.Configs {
		c := &env.Configs[i]
		if c.ID == configID {
			if c.Kind != core.EnvConfigKindLocalhost {
				return fmt.Errorf("配置 %s 类型 %s 不是 localhost", configID, c.Kind)
			}
			return nil
		}
	}
	return fmt.Errorf("配置未找到: %s", configID)
}
