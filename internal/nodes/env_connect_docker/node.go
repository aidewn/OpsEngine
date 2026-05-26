// env_connect_docker 节点：从环境配置读取 docker 配置并建立 Docker 客户端
// Phase 3 仅支持 mode=over_ssh：从同环境内引用的 ssh 配置拨号 → SSH 隧道 → Docker daemon
// docker_connect 节点保留，作为「凭证内联」的临时方案

package env_connect_docker

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

const defaultSocketPath = "/var/run/docker.sock"

func init() { engine.Register(&Node{}) }

// Node env_connect_docker 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "env_connect_docker",
		DisplayName: "环境 · Docker 连接",
		Category:    "environment",
		NodeKind:    core.NodeKindAction,
		Icon:        "🐳",
		Description: "从环境配置读取 Docker 凭证并建立连接，输出 DockerContext 句柄",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "client", Label: "Docker", PortType: core.PortTypeDockerContext},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "env_select", ID: "environment_id", Label: "环境", Required: true},
			{Type: "env_config_select", ID: "config_id", Label: "Docker 配置",
				Required: true, ConfigKindFilter: string(core.EnvConfigKindDocker)},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 解析环境 → 找 docker 配置 → 按 mode 建连
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

	dockerCfg, err := findDockerConfig(env, configID)
	if err != nil {
		return nil, err
	}

	mode := strings.TrimSpace(stringField(dockerCfg.Fields, "mode"))
	if mode == "" {
		mode = "over_ssh"
	}
	if mode != "over_ssh" {
		return nil, fmt.Errorf("暂不支持 Docker mode=%s（Phase 3 仅 over_ssh）", mode)
	}

	sshConfigID := strings.TrimSpace(stringField(dockerCfg.Fields, "ssh_config_id"))
	if sshConfigID == "" {
		return nil, fmt.Errorf("Docker 配置缺少 ssh_config_id")
	}
	sshCfg, err := findSSHConfig(env, sshConfigID)
	if err != nil {
		return nil, err
	}

	socketPath := strings.TrimSpace(stringField(dockerCfg.Fields, "socket_path"))
	if socketPath == "" {
		socketPath = defaultSocketPath
	}

	host := strings.TrimSpace(stringField(sshCfg.Fields, "host"))
	user := strings.TrimSpace(stringField(sshCfg.Fields, "user"))
	password := stringField(sshCfg.Fields, "password")
	port := intField(sshCfg.Fields, "port", 22)
	timeout := intField(sshCfg.Fields, "timeout_seconds", 10)
	if host == "" || user == "" || password == "" {
		return nil, fmt.Errorf("引用的 SSH 配置缺少 host/user/password")
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	ctx.Info("正在通过 SSH %s@%s 拨号到 Docker socket %s", user, addr, socketPath)

	linuxClient, err := clients.DialLinuxSsh(host, port, user, password, timeout)
	if err != nil {
		return nil, err
	}

	// DockerClient 取得底层 ssh.Client 的所有权（Close 会一起关）
	// 因此不再调用 linuxClient.Close，避免双重关闭
	dockerClient, err := clients.NewDockerClientOverSSH(linuxClient.Client(), host, port, user, socketPath)
	if err != nil {
		// 包装失败时手动关闭 SSH，避免泄漏
		_ = linuxClient.Close()
		return nil, fmt.Errorf("构造 Docker 客户端失败: %w", err)
	}

	if err := dockerClient.Ping(ctx.Context()); err != nil {
		_ = dockerClient.Close()
		return nil, fmt.Errorf("Docker daemon 不可达: %w", err)
	}

	ctx.Info("Docker 连接成功: %s@%s (socket=%s)", user, addr, socketPath)
	return engine.Outputs{
		"client": dockerClient,
	}, nil
}

// findDockerConfig 在环境中按 configID 定位 docker 配置
func findDockerConfig(env core.EnvironmentDef, configID string) (*core.EnvConfigItem, error) {
	for i := range env.Configs {
		c := &env.Configs[i]
		if c.ID == configID {
			if c.Kind != core.EnvConfigKindDocker {
				return nil, fmt.Errorf("配置 %s 类型 %s 不是 docker", configID, c.Kind)
			}
			return c, nil
		}
	}
	return nil, fmt.Errorf("配置未找到: %s", configID)
}

// findSSHConfig 在环境中按 ID 定位 ssh 配置（用于 docker over_ssh 反向引用）
func findSSHConfig(env core.EnvironmentDef, configID string) (*core.EnvConfigItem, error) {
	for i := range env.Configs {
		c := &env.Configs[i]
		if c.ID == configID {
			if c.Kind != core.EnvConfigKindSSH {
				return nil, fmt.Errorf("ssh_config_id %s 类型 %s 不是 ssh", configID, c.Kind)
			}
			return c, nil
		}
	}
	return nil, fmt.Errorf("ssh_config_id 未找到: %s", configID)
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
