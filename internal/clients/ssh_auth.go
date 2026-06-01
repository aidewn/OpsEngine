package clients

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	// SSHAuthPassword 使用账号密码认证，兼容已有环境配置。
	SSHAuthPassword = "password"
	// SSHAuthPrivateKey 使用本机私钥文件认证，配置中只保存文件路径。
	SSHAuthPrivateKey = "private_key"
)

// LinuxSshDialConfig 描述一次 SSH 拨号所需的最小凭据。
// 私钥认证只保存本机路径，私钥内容不进入环境配置和执行记录。
type LinuxSshDialConfig struct {
	Host                 string
	Port                 int
	User                 string
	AuthType             string
	Password             string
	PrivateKeyPath       string
	PrivateKeyPassphrase string
	TimeoutSeconds       int
}

// ParseLinuxSshDialConfig 从环境配置 fields 中解析 SSH 拨号配置。
// auth_type 为空时按 password 处理，以兼容已有数据。
func ParseLinuxSshDialConfig(fields map[string]any) (LinuxSshDialConfig, error) {
	cfg := LinuxSshDialConfig{
		Host:                 strings.TrimSpace(stringMapField(fields, "host")),
		Port:                 intMapField(fields, "port", 22),
		User:                 strings.TrimSpace(stringMapField(fields, "user")),
		AuthType:             strings.TrimSpace(stringMapField(fields, "auth_type")),
		Password:             stringMapField(fields, "password"),
		PrivateKeyPath:       strings.TrimSpace(stringMapField(fields, "private_key_path")),
		PrivateKeyPassphrase: stringMapField(fields, "private_key_passphrase"),
		TimeoutSeconds:       intMapField(fields, "timeout_seconds", 10),
	}
	if cfg.AuthType == "" {
		cfg.AuthType = SSHAuthPassword
	}
	if err := cfg.Validate(); err != nil {
		return LinuxSshDialConfig{}, err
	}
	return cfg, nil
}

// Validate 校验 SSH 配置完整性，不做网络访问。
func (c LinuxSshDialConfig) Validate() error {
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("SSH 配置缺少 host")
	}
	if strings.TrimSpace(c.User) == "" {
		return fmt.Errorf("SSH 配置缺少 user")
	}
	if c.Port <= 0 {
		return fmt.Errorf("SSH 配置 port 非法")
	}
	if c.TimeoutSeconds <= 0 {
		return fmt.Errorf("SSH 配置 timeout_seconds 非法")
	}
	switch c.AuthType {
	case SSHAuthPassword:
		if c.Password == "" {
			return fmt.Errorf("SSH 密码认证缺少 password")
		}
	case SSHAuthPrivateKey:
		if strings.TrimSpace(c.PrivateKeyPath) == "" {
			return fmt.Errorf("SSH 密钥认证缺少 private_key_path")
		}
	default:
		return fmt.Errorf("SSH auth_type %q 不支持", c.AuthType)
	}
	return nil
}

// Dial 按配置拨号 SSH，成功后包装为 LinuxSshClient 句柄。
func (c LinuxSshDialConfig) Dial() (*LinuxSshClient, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	auth, err := c.authMethod()
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	config := &ssh.ClientConfig{
		User: c.User,
		Auth: []ssh.AuthMethod{auth},
		// MVP 阶段先跳过 host key 校验，后续可扩展 known_hosts / 指纹配置。
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Duration(c.TimeoutSeconds) * time.Second,
	}
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败: %w", err)
	}
	return NewLinuxSshClient(client, c.Host, c.Port, c.User), nil
}

// authMethod 根据认证方式构造 Go SSH 认证对象。
func (c LinuxSshDialConfig) authMethod() (ssh.AuthMethod, error) {
	switch c.AuthType {
	case SSHAuthPassword:
		return ssh.Password(c.Password), nil
	case SSHAuthPrivateKey:
		path, err := expandLocalPath(c.PrivateKeyPath)
		if err != nil {
			return nil, err
		}
		key, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取 SSH 私钥失败: %w", err)
		}
		var signer ssh.Signer
		if c.PrivateKeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(c.PrivateKeyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(key)
		}
		if err != nil {
			return nil, fmt.Errorf("解析 SSH 私钥失败: %w", err)
		}
		return ssh.PublicKeys(signer), nil
	default:
		return nil, fmt.Errorf("SSH auth_type %q 不支持", c.AuthType)
	}
}

func stringMapField(fields map[string]any, key string) string {
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

func intMapField(fields map[string]any, key string, fallback int) int {
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
	case float32:
		return int(n)
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case json.Number:
		i, err := n.Int64()
		if err == nil {
			return int(i)
		}
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err == nil {
			return i
		}
	}
	return fallback
}

func expandLocalPath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	if p == "~" || strings.HasPrefix(p, "~"+string(filepath.Separator)) || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("解析用户主目录失败: %w", err)
		}
		if p == "~" {
			return home, nil
		}
		p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~\\"), "~/"))
	}
	return os.ExpandEnv(p), nil
}
