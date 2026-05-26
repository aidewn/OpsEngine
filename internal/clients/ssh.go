// SSH 客户端运行时句柄
// 句柄可在同一次执行中通过 LinuxSshConnection 端口传递，但序列化时只暴露安全元数据
// 内部按需 lazy 打开 SFTP 子系统，供文件类节点复用同一条连接

package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// WalkSFTPMatch 通过 SFTP Walk 在 startDir 下按正则匹配 basename，遵守最大深度
// 与 linux_find_file 节点 / env_probe_ssh_find_files 探测共用同一份算法
// onWarn 用于把遍历过程中的非致命错误传给调用方（节点日志 / 前端探测忽略）
// ctx 用于响应取消（用户停止）；ctx 可为 nil 时表示不可取消
func WalkSFTPMatch(
	ctx context.Context,
	client *LinuxSshClient,
	startDir string,
	re *regexp.Regexp,
	maxDepth int,
	onWarn func(format string, args ...any),
) ([]string, error) {
	if client == nil {
		return nil, fmt.Errorf("LinuxSshConnection 未连接")
	}
	if re == nil {
		return nil, fmt.Errorf("正则不能为空")
	}
	if maxDepth <= 0 {
		maxDepth = 5
	}
	sc, err := client.Sftp()
	if err != nil {
		return nil, err
	}

	cleanedStart := path.Clean(startDir)
	if cleanedStart == "" {
		cleanedStart = "/"
	}
	startDepth := strings.Count(cleanedStart, "/")
	var matches []string
	walker := sc.Walk(cleanedStart)
	for walker.Step() {
		if walker.Err() != nil {
			if onWarn != nil {
				onWarn("遍历跳过: %v", walker.Err())
			}
			continue
		}
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		current := walker.Path()
		depth := strings.Count(path.Clean(current), "/") - startDepth
		if depth > maxDepth {
			walker.SkipDir()
			continue
		}
		info := walker.Stat()
		if info == nil || info.IsDir() {
			continue
		}
		if re.MatchString(info.Name()) {
			matches = append(matches, current)
		}
	}
	return matches, nil
}

// DialLinuxSsh 用账号密码拨号 SSH，成功后包装为 LinuxSshClient 句柄
// 调用方负责传入校验过的参数（host/user/password 非空、port/timeoutSeconds > 0）
// 错误统一以 "SSH 连接失败: %w" 包装，便于上层一致呈现
func DialLinuxSsh(host string, port int, user, password string, timeoutSeconds int) (*LinuxSshClient, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		// MVP 阶段先跳过 host key 校验，后续可扩展 known_hosts / 指纹配置
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Duration(timeoutSeconds) * time.Second,
	}
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败: %w", err)
	}
	return NewLinuxSshClient(client, host, port, user), nil
}

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
