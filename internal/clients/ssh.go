// SSH 客户端运行时句柄
// 句柄可在同一次执行中通过 LinuxSshConnection 端口传递，但序列化时只暴露安全元数据
// 内部按需 lazy 打开 SFTP 子系统，供文件类节点复用同一条连接

package clients

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// LinuxSshClient Linux SSH 连接句柄
// client / sftpClient 是真实连接对象，不参与 JSON 序列化，避免泄露内部状态或导致执行记录无法持久化
type LinuxSshClient struct {
	client      *ssh.Client
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	User        string    `json:"user"`
	ConnectedAt time.Time `json:"connected_at"`

	// SFTP 子系统按需创建并缓存，整次执行复用，节省握手开销
	sftpMu     sync.Mutex
	sftpClient *sftp.Client
}

// NewLinuxSshClient 创建 SSH 运行时句柄
func NewLinuxSshClient(client *ssh.Client, host string, port int, user string) *LinuxSshClient {
	return &LinuxSshClient{
		client:      client,
		Host:        host,
		Port:        port,
		User:        user,
		ConnectedAt: time.Now(),
	}
}

// Client 返回底层 SSH 客户端，供后续执行命令节点复用
func (c *LinuxSshClient) Client() *ssh.Client {
	if c == nil {
		return nil
	}
	return c.client
}

// Sftp 返回当前连接上的 SFTP 子系统客户端，首次调用时建立
// 调用方不要单独 Close，统一由 LinuxSshClient.Close 负责
func (c *LinuxSshClient) Sftp() (*sftp.Client, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("LinuxSshConnection 未连接")
	}
	c.sftpMu.Lock()
	defer c.sftpMu.Unlock()
	if c.sftpClient != nil {
		return c.sftpClient, nil
	}
	sc, err := sftp.NewClient(c.client)
	if err != nil {
		return nil, fmt.Errorf("打开 SFTP 子系统失败: %w", err)
	}
	c.sftpClient = sc
	return sc, nil
}

// Close 关闭底层 SSH 连接（含 SFTP 子系统）
func (c *LinuxSshClient) Close() error {
	if c == nil {
		return nil
	}
	c.sftpMu.Lock()
	if c.sftpClient != nil {
		_ = c.sftpClient.Close()
		c.sftpClient = nil
	}
	c.sftpMu.Unlock()
	if c.client == nil {
		return nil
	}
	return c.client.Close()
}

// MarshalJSON 只输出安全元数据，不输出密码、私钥或底层连接对象
func (c *LinuxSshClient) MarshalJSON() ([]byte, error) {
	if c == nil {
		return []byte("null"), nil
	}
	type safeView struct {
		Type        string    `json:"type"`
		Host        string    `json:"host"`
		Port        int       `json:"port"`
		User        string    `json:"user"`
		Connected   bool      `json:"connected"`
		ConnectedAt time.Time `json:"connected_at"`
	}
	return json.Marshal(safeView{
		Type:        "LinuxSshConnection",
		Host:        c.Host,
		Port:        c.Port,
		User:        c.User,
		Connected:   c.client != nil,
		ConnectedAt: c.ConnectedAt,
	})
}
