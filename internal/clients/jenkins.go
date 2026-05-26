// Jenkins 运行时句柄
// 使用 net/http + Basic Auth (user + API token) 直连 Jenkins REST API
// 不引入额外 Jenkins SDK，保持依赖最小
// 句柄可在同一次执行中通过 JenkinsContext 端口传递，序列化时只暴露安全元数据

package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// JenkinsClient Jenkins API 运行时句柄
// http.Client 复用 keepalive；token 仅保存在内存中
type JenkinsClient struct {
	http     *http.Client
	BaseURL  string    `json:"base_url"`
	User     string    `json:"user"`
	token    string    // API token，不序列化
	ConnectedAt time.Time `json:"connected_at"`
}

// JenkinsJob Jenkins job 的精简视图（探测节点 / 列表节点 共用）
type JenkinsJob struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Color string `json:"color"`           // Jenkins 用 color 表征状态（blue=success / red=failed / disabled / 等）
	Class string `json:"_class,omitempty"` // 是否为 Folder：org.jenkinsci.plugins.workflow.multibranch.WorkflowMultiBranchProject 等
}

// NewJenkinsClient 构造客户端；不立即建连，调用方应随后 Ping 验证
// timeoutSeconds <= 0 时默认 10s
func NewJenkinsClient(baseURL, user, apiToken string, timeoutSeconds int) (*JenkinsClient, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("Jenkins base_url 为空")
	}
	if strings.TrimSpace(user) == "" {
		return nil, fmt.Errorf("Jenkins user 为空")
	}
	if strings.TrimSpace(apiToken) == "" {
		return nil, fmt.Errorf("Jenkins api_token 为空")
	}
	// 校验 URL；统一去掉尾部斜杠以便拼接 /api/json
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("Jenkins base_url 非法: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("Jenkins base_url 必须以 http:// 或 https:// 开头")
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 10
	}
	return &JenkinsClient{
		http:        &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
		BaseURL:     parsed.String(),
		User:        user,
		token:       apiToken,
		ConnectedAt: time.Now(),
	}, nil
}

// Ping 用 /api/json 验证连接与凭证；非 2xx 状态码即视为不可达
func (c *JenkinsClient) Ping(ctx context.Context) error {
	if c == nil || c.http == nil {
		return fmt.Errorf("Jenkins 客户端未初始化")
	}
	resp, err := c.do(ctx, "GET", "/api/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 401/403 多半是 token 不对；500 多半是 Jenkins 自身故障
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("Jenkins 不可达 status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// ListJobs 列出指定 folder 下的 job
// folder 为空 → Jenkins 根目录；可写嵌套路径如 "foo/bar"
// 返回值即调用方需要的精简 job 列表
func (c *JenkinsClient) ListJobs(ctx context.Context, folder string) ([]JenkinsJob, error) {
	if c == nil || c.http == nil {
		return nil, fmt.Errorf("Jenkins 客户端未初始化")
	}
	// /job/foo/job/bar/api/json?tree=jobs[name,url,color,_class]
	path := buildFolderPath(folder) + "/api/json"
	q := url.Values{}
	q.Set("tree", "jobs[name,url,color,_class]")
	resp, err := c.do(ctx, "GET", path+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("Jenkins 路径不存在: %s", folder)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("ListJobs 失败 status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Jobs []JenkinsJob `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析 Jenkins 响应失败: %w", err)
	}
	return payload.Jobs, nil
}

// do 内部 HTTP 调用：自动加 Basic Auth + JSON Accept
// path 应以 / 开头（或形如 ?query=...）
func (c *JenkinsClient) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	fullURL := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.User, c.token)
	req.Header.Set("Accept", "application/json")
	return c.http.Do(req)
}

// buildFolderPath 把 "foo/bar" 转成 "/job/foo/job/bar"；空 folder 返回 ""（指向 Jenkins 根）
func buildFolderPath(folder string) string {
	parts := strings.Split(strings.Trim(folder, "/"), "/")
	var out strings.Builder
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out.WriteString("/job/")
		out.WriteString(url.PathEscape(p))
	}
	return out.String()
}

// MarshalJSON 只输出安全元数据，不暴露 api_token
func (c *JenkinsClient) MarshalJSON() ([]byte, error) {
	if c == nil {
		return []byte("null"), nil
	}
	type safeView struct {
		Type        string    `json:"type"`
		BaseURL     string    `json:"base_url"`
		User        string    `json:"user"`
		Connected   bool      `json:"connected"`
		ConnectedAt time.Time `json:"connected_at"`
	}
	return json.Marshal(safeView{
		Type:        "JenkinsContext",
		BaseURL:     c.BaseURL,
		User:        c.User,
		Connected:   c.http != nil,
		ConnectedAt: c.ConnectedAt,
	})
}
