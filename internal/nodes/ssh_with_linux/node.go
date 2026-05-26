// ssh_with_linux 节点：用配置中的账号密码建立 Linux SSH 连接

package ssh_with_linux

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

// Node ssh_with_linux 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	minPort, maxPort := int64(1), int64(65535)
	minTimeout, maxTimeout := int64(1), int64(300)
	return core.NodeTypeDef{
		TypeID:      "ssh_with_linux",
		DisplayName: "SSH Linux 连接",
		Category:    "remote",
		NodeKind:    core.NodeKindAction,
		Icon:        "🔐",
		Description: "使用 IP、端口、用户名和密码建立 Linux SSH 连接",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "client", Label: "SSH", PortType: core.PortTypeLinuxSsh},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "host", Label: "IP / Host", Placeholder: "192.168.1.10", Required: true},
			{Type: "number", ID: "port", Label: "SSH 端口", Required: true,
				Min: &minPort, Max: &maxPort, Default: int64(defaultPort)},
			{Type: "text", ID: "user", Label: "用户名", Placeholder: "root", Required: true},
			{Type: "password", ID: "password", Label: "密码", Required: true},
			{Type: "number", ID: "timeout_seconds", Label: "超时（秒）",
				Min: &minTimeout, Max: &maxTimeout, Default: int64(defaultTimeoutSeconds)},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 立即建立 SSH 连接，成功后输出 LinuxSshConnection 运行时句柄
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	host := strings.TrimSpace(ctx.ConfigString("host"))
	user := strings.TrimSpace(ctx.ConfigString("user"))
	password := ctx.ConfigString("password")
	port := int(ctx.ConfigInt("port"))
	timeoutSeconds := ctx.ConfigInt("timeout_seconds")

	if host == "" {
		return nil, fmt.Errorf("ssh_with_linux 节点的 host 未配置")
	}
	if user == "" {
		return nil, fmt.Errorf("ssh_with_linux 节点的 user 未配置")
	}
	if password == "" {
		return nil, fmt.Errorf("ssh_with_linux 节点的 password 未配置")
	}
	if port == 0 {
		port = defaultPort
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultTimeoutSeconds
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	ctx.Info("正在连接 SSH %s@%s", user, addr)

	client, err := clients.DialLinuxSsh(host, port, user, password, int(timeoutSeconds))
	if err != nil {
		return nil, err
	}

	ctx.Info("SSH 连接成功: %s@%s", user, addr)
	return engine.Outputs{
		"client": client,
	}, nil
}
