// 镜像仓库（Docker Registry v2）辅助：
//   - PingRegistry：GET /v2/ 探活（200 / 401 视为成功；其它视为失败）
//   - BuildRegistryAuth：把 user/password 序列化成 docker SDK 需要的 base64(json) 凭据头
//   - NormalizeRegistryHost：从 https://reg.example.com:5000/ → reg.example.com:5000，
//     方便作为镜像 tag 的前缀（registry.example.com/repo:tag）
//
// 设计取舍：不引入额外 SDK（distribution / go-containerregistry），用 net/http + base64 直拼，
// 与 internal/clients/jenkins.go 的极简风格保持一致

package clients

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	registrytypes "github.com/docker/docker/api/types/registry"
)

// PingRegistry 请求 <baseURL>/v2/
//
//	HTTP 200            → 匿名访问 OK，配置可用
//	HTTP 401            → 仓库要求认证；若 user/password 已填且仍 401，说明凭据错；
//	                      若 user/password 为空，则告知调用方需要填凭据
//	HTTP 其它 / 拨号失败 → 直接报错
func PingRegistry(rawURL, user, password string, timeout time.Duration) error {
	endpoint, err := buildPingURL(rawURL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("构造 registry 请求失败: %w", err)
	}
	if user != "" {
		req.SetBasicAuth(user, password)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("连接 registry 失败: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		if user == "" {
			return fmt.Errorf("registry 要求认证，请填写 user / password")
		}
		return fmt.Errorf("registry 认证失败（401）：请检查 user / password")
	default:
		return fmt.Errorf("registry 探活失败: HTTP %d", resp.StatusCode)
	}
}

// BuildRegistryAuth 把 user/password/serveraddress 打包为 docker SDK 所需的 X-Registry-Auth header 值
// 用于 ImagePush 等需要鉴权的 SDK 调用；匿名仓库可传空，返回值为空字符串
func BuildRegistryAuth(user, password, serverAddress string) (string, error) {
	if user == "" && password == "" {
		return "", nil
	}
	cfg := registrytypes.AuthConfig{
		Username:      user,
		Password:      password,
		ServerAddress: serverAddress,
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("序列化 registry auth 失败: %w", err)
	}
	return base64.URLEncoding.EncodeToString(raw), nil
}

// NormalizeRegistryHost 从用户填的 url 中抽出 host[:port]，用作镜像 tag 前缀
//   - "https://reg.example.com:5000"  → "reg.example.com:5000"
//   - "reg.example.com"               → "reg.example.com"
//   - "reg.example.com/"              → "reg.example.com"
func NormalizeRegistryHost(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("registry url 为空")
	}
	// 用户可能不带 scheme，补一个临时 https 便于 url.Parse 解析 host
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("解析 registry url 失败: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("registry url 缺少 host: %s", rawURL)
	}
	return u.Host, nil
}

// buildPingURL 把任意形式的 registry url 整理为 <scheme>://<host>/v2/
// 用户可能填了 path（如 /v2/ 或仓库子路径），这里统一覆盖为 /v2/
func buildPingURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("registry url 为空")
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("解析 registry url 失败: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("registry url 缺少 host: %s", rawURL)
	}
	u.Path = "/v2/"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}
