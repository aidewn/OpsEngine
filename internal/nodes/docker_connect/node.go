// docker_connect 节点：通过 SSH 隧道连接远端 Docker daemon
// 自己建立 SSH 连接（不复用 ssh_with_linux 输出的 LinuxSshConnection），
// 隧道直连远端 /var/run/docker.sock，无需开放 2375 TCP 端口

package docker_connect

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	"golang.org/x/crypto/ssh"
)

const (
	defaultSSHPort        = 22
	defaultTimeoutSeconds = 10
	defaultSocketPath     = "/var/run/docker.sock"
)

func init() { engine.Register(&Node{}) }

// Node docker_connect 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	minPort, maxPort := int64(1), int64(65535)
	minTimeout, maxTimeout := int64(1), int64(300)
	return core.NodeTypeDef{
		TypeID:      "docker_connect",
		DisplayName: "Docker 连接",
		Category:    "remote",
		NodeKind:    core.NodeKindAction,
		Icon:        "🐳",
		Description: "通过 SSH 隧道连接远端 Docker daemon（/var/run/docker.sock），输出 DockerContext",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "client", Label: "Docker", PortType: core.PortTypeDockerContext},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "host", Label: "IP / Host", Placeholder: "192.168.1.10", Required: true},
			{Type: "number", ID: "port", Label: "SSH 端口", Required: true,
				Min: &minPort, Max: &maxPort, Default: int64(defaultSSHPort)},
			{Type: "text", ID: "user", Label: "SSH 用户名", Placeholder: "root", Required: true},
			{Type: "password", ID: "password", Label: "SSH 密码", Required: true},
			{Type: "text", ID: "socket_path", Label: "Docker socket 路径",
				Placeholder: defaultSocketPath, Default: defaultSocketPath},
			{Type: "number", ID: "timeout_seconds", Label: "SSH 超时（秒）",
				Min: &minTimeout, Max: &maxTimeout, Default: int64(defaultTimeoutSeconds)},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 建立 SSH 连接 → 用其作为 Docker API 的传输层 → ping daemon 验证可达
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	host := strings.TrimSpace(ctx.ConfigString("host"))
	user := strings.TrimSpace(ctx.ConfigString("user"))
	password := ctx.ConfigString("password")
	port := int(ctx.ConfigInt("port"))
	socketPath := strings.TrimSpace(ctx.ConfigString("socket_path"))
	timeoutSeconds := ctx.ConfigInt("timeout_seconds")

	if host == "" {
		return nil, fmt.Errorf("docker_connect 节点的 host 未配置")
	}
	if user == "" {
		return nil, fmt.Errorf("docker_connect 节点的 user 未配置")
	}
	if password == "" {
		return nil, fmt.Errorf("docker_connect 节点的 password 未配置")
	}
	if port == 0 {
		port = defaultSSHPort
	}
	if socketPath == "" {
		socketPath = defaultSocketPath
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultTimeoutSeconds
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	ctx.Info("建立 SSH 隧道 %s@%s → docker socket %s", user, addr, socketPath)

	sshConfig := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Duration(timeoutSeconds) * time.Second,
	}
	sshClient, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败: %w", err)
	}

	dockerClient, err := clients.NewDockerClientOverSSH(sshClient, host, port, user, socketPath)
	if err != nil {
		_ = sshClient.Close()
		return nil, err
	}

	// 探活：daemon 不可达（socket 不存在/权限不足）尽早暴露
	if err := dockerClient.Ping(ctx.Context()); err != nil {
		_ = dockerClient.Close()
		return nil, fmt.Errorf("Docker daemon 不可达: %w", err)
	}

	ctx.Info("Docker 连接成功: %s@%s (%s)", user, addr, socketPath)
	return engine.Outputs{
		"client": dockerClient,
	}, nil
}
