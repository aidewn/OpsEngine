// Prometheus HTTP API 客户端。
// 仅封装 OpsEngine 监控源需要的轻量查询能力，不保存时序数据。

package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// PrometheusConfig 是 Prometheus 监控源连接配置。
type PrometheusConfig struct {
	Endpoint       string
	AuthType       string
	Username       string
	Password       string
	Token          string
	TimeoutSeconds int
}

// PrometheusClient 负责请求 Prometheus HTTP API。
type PrometheusClient struct {
	config PrometheusConfig
	client *http.Client
}

// PrometheusSampleValue 是一条 Prometheus vector 结果。
type PrometheusSampleValue struct {
	Metric map[string]string `json:"metric"`
	Value  float64           `json:"value"`
}

// PrometheusQueryResult 是即时查询的归一化结果。
type PrometheusQueryResult struct {
	Query  string                  `json:"query"`
	Value  float64                 `json:"value"`
	Values []PrometheusSampleValue `json:"values"`
}

// NewPrometheusClient 从 map 配置构造客户端。
func NewPrometheusClient(config map[string]any) (*PrometheusClient, error) {
	cfg := PrometheusConfig{
		Endpoint:       strings.TrimRight(mapString(config, "endpoint"), "/"),
		AuthType:       mapString(config, "auth_type"),
		Username:       mapString(config, "username"),
		Password:       mapString(config, "password"),
		Token:          mapString(config, "token"),
		TimeoutSeconds: mapInt(config, "timeout_seconds", 10),
	}
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("Prometheus endpoint 不能为空")
	}
	if cfg.AuthType == "" {
		cfg.AuthType = "none"
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 10
	}
	return &PrometheusClient{
		config: cfg,
		client: &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
	}, nil
}

// Query 执行 /api/v1/query 即时查询。
func (c *PrometheusClient) Query(ctx context.Context, query string, at string) (PrometheusQueryResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return PrometheusQueryResult{}, fmt.Errorf("PromQL 不能为空")
	}
	u, err := url.Parse(c.config.Endpoint + "/api/v1/query")
	if err != nil {
		return PrometheusQueryResult{}, fmt.Errorf("Prometheus endpoint 无效: %w", err)
	}
	q := u.Query()
	q.Set("query", query)
	if strings.TrimSpace(at) != "" {
		q.Set("time", strings.TrimSpace(at))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return PrometheusQueryResult{}, err
	}
	c.applyAuth(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return PrometheusQueryResult{}, fmt.Errorf("请求 Prometheus 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PrometheusQueryResult{}, fmt.Errorf("Prometheus 返回状态码 %d", resp.StatusCode)
	}

	var raw prometheusAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return PrometheusQueryResult{}, fmt.Errorf("解析 Prometheus 响应失败: %w", err)
	}
	if raw.Status != "success" {
		msg := raw.Error
		if msg == "" {
			msg = raw.ErrorType
		}
		if msg == "" {
			msg = "未知错误"
		}
		return PrometheusQueryResult{}, fmt.Errorf("Prometheus 查询失败: %s", msg)
	}
	return normalizePrometheusQueryResult(query, raw.Data.Result)
}

// Ping 用 up 查询测试 Prometheus 可达性和认证。
func (c *PrometheusClient) Ping(ctx context.Context) error {
	_, err := c.Query(ctx, "up", "")
	return err
}

// applyAuth 按配置注入认证头。
func (c *PrometheusClient) applyAuth(req *http.Request) {
	switch c.config.AuthType {
	case "basic":
		req.SetBasicAuth(c.config.Username, c.config.Password)
	case "bearer":
		if c.config.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.config.Token)
		}
	}
}

// prometheusAPIResponse 是 Prometheus HTTP API 响应结构。
type prometheusAPIResponse struct {
	Status    string            `json:"status"`
	ErrorType string            `json:"errorType"`
	Error     string            `json:"error"`
	Data      prometheusAPIData `json:"data"`
}

// prometheusAPIData 是响应 data 字段。
type prometheusAPIData struct {
	ResultType string                `json:"resultType"`
	Result     []prometheusAPIResult `json:"result"`
}

// prometheusAPIResult 是 vector/matrix 结果项；第一阶段只处理 vector value。
type prometheusAPIResult struct {
	Metric map[string]string `json:"metric"`
	Value  []any             `json:"value"`
}

// normalizePrometheusQueryResult 将 Prometheus 字符串数值转为 float64。
func normalizePrometheusQueryResult(query string, items []prometheusAPIResult) (PrometheusQueryResult, error) {
	out := PrometheusQueryResult{Query: query, Values: []PrometheusSampleValue{}}
	for _, item := range items {
		if len(item.Value) < 2 {
			continue
		}
		valueText, _ := item.Value[1].(string)
		value, err := strconv.ParseFloat(valueText, 64)
		if err != nil {
			return PrometheusQueryResult{}, fmt.Errorf("Prometheus 样本值不是数字: %s", valueText)
		}
		out.Values = append(out.Values, PrometheusSampleValue{Metric: item.Metric, Value: value})
	}
	if len(out.Values) > 0 {
		out.Value = out.Values[0].Value
	}
	return out, nil
}

// mapString 读取字符串配置。
func mapString(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	if v, ok := config[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// mapInt 读取整数配置，兼容 JSON 解码后的 float64。
func mapInt(config map[string]any, key string, fallback int) int {
	if config == nil {
		return fallback
	}
	switch v := config[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return fallback
}
