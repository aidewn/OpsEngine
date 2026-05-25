// Docker 运行时句柄
// 通过 SSH 隧道连接远端 Docker daemon 的 /var/run/docker.sock，不要求开放 2375 TCP
// 句柄可在同一次执行中通过 DockerContext 端口传递，序列化时只暴露安全元数据

package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	dockerclient "github.com/docker/docker/client"
	"golang.org/x/crypto/ssh"
)

// DockerClient Docker 运行时句柄
// 内部持有 SSH 隧道 + Docker API client；Close 会一并关闭两者
type DockerClient struct {
	api       *dockerclient.Client
	sshClient *ssh.Client

	Host        string    `json:"host"`
	Port        int       `json:"port"`
	User        string    `json:"user"`
	SocketPath  string    `json:"socket_path"`
	ConnectedAt time.Time `json:"connected_at"`

	closeMu sync.Mutex
	closed  bool
}

// NewDockerClientOverSSH 基于已建立的 SSH 连接构造 Docker 客户端
// socketPath 默认 /var/run/docker.sock，可由配置覆盖（部分发行版用 /run/docker.sock）
func NewDockerClientOverSSH(sshClient *ssh.Client, host string, port int, user, socketPath string) (*DockerClient, error) {
	if sshClient == nil {
		return nil, fmt.Errorf("SSH 连接为空")
	}
	if socketPath == "" {
		socketPath = "/var/run/docker.sock"
	}

	// 每次 HTTP 请求都通过 SSH 拨号到远端 Unix socket
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return sshClient.Dial("unix", socketPath)
			},
		},
	}

	api, err := dockerclient.NewClientWithOpts(
		dockerclient.WithHTTPClient(httpClient),
		dockerclient.WithHost("http://docker"), // dummy host，实际由 DialContext 接管
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("构造 Docker 客户端失败: %w", err)
	}

	return &DockerClient{
		api:         api,
		sshClient:   sshClient,
		Host:        host,
		Port:        port,
		User:        user,
		SocketPath:  socketPath,
		ConnectedAt: time.Now(),
	}, nil
}

// API 返回底层 Docker SDK 客户端，供后续 docker_* 操作节点使用
func (c *DockerClient) API() *dockerclient.Client {
	if c == nil {
		return nil
	}
	return c.api
}

// Ping 测试 Docker daemon 可达性，确认 SSH 隧道工作正常
func (c *DockerClient) Ping(ctx context.Context) error {
	if c == nil || c.api == nil {
		return fmt.Errorf("Docker 客户端未初始化")
	}
	_, err := c.api.Ping(ctx)
	return err
}

// Close 关闭 Docker 客户端 + SSH 隧道
func (c *DockerClient) Close() error {
	if c == nil {
		return nil
	}
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if c.api != nil {
		_ = c.api.Close()
	}
	if c.sshClient != nil {
		return c.sshClient.Close()
	}
	return nil
}

// MarshalJSON 只输出安全元数据，不暴露底层连接
func (c *DockerClient) MarshalJSON() ([]byte, error) {
	if c == nil {
		return []byte("null"), nil
	}
	type safeView struct {
		Type        string    `json:"type"`
		Host        string    `json:"host"`
		Port        int       `json:"port"`
		User        string    `json:"user"`
		SocketPath  string    `json:"socket_path"`
		Connected   bool      `json:"connected"`
		ConnectedAt time.Time `json:"connected_at"`
	}
	return json.Marshal(safeView{
		Type:        "DockerContext",
		Host:        c.Host,
		Port:        c.Port,
		User:        c.User,
		SocketPath:  c.SocketPath,
		Connected:   c.api != nil && !c.closed,
		ConnectedAt: c.ConnectedAt,
	})
}
