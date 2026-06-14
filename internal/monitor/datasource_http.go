// http.health 数据源：对一个 URL 发一次探活，返回状态码与响应时间。
// 无需 SSH/环境凭证，是最自包含的一类采集。

package monitor

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// KindHTTPHealth 是 http.health 的数据类型标识。
const KindHTTPHealth = "http.health"

// HTTPHealthResult 是一次 HTTP 探活的结构化结果。
type HTTPHealthResult struct {
	URL        string `json:"url"`
	Method     string `json:"method"`
	StatusCode int    `json:"status_code"`
	LatencyMS  int64  `json:"latency_ms"`
	OK         bool   `json:"ok"` // 是否满足期望状态码（默认 2xx）
}

// httpHealthSource 实现 DataSource。client 可注入便于测试。
type httpHealthSource struct {
	client *http.Client
}

// newHTTPHealthSource 构造数据源；client 为 nil 时按任务超时新建。
func newHTTPHealthSource() *httpHealthSource { return &httpHealthSource{} }

func (s *httpHealthSource) Kind() string { return KindHTTPHealth }

// Collect 参数：url（必填）、method（默认 GET）、timeout_seconds（默认 10）、
// expect_status（可选，指定则严格相等，否则按 2xx 判定 OK）。
func (s *httpHealthSource) Collect(ctx context.Context, cc CollectContext) (any, error) {
	url := paramString(cc.Task.Params, "url")
	if url == "" {
		return nil, fmt.Errorf("http.health 缺少 url 参数")
	}
	method := paramString(cc.Task.Params, "method")
	if method == "" {
		method = http.MethodGet
	}
	timeout := paramInt(cc.Task.Params, "timeout_seconds", 10)

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}

	client := s.client
	if client == nil {
		client = &http.Client{Timeout: time.Duration(timeout) * time.Second}
	}

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	expect := paramInt(cc.Task.Params, "expect_status", 0)
	return HTTPHealthResult{
		URL:        url,
		Method:     method,
		StatusCode: resp.StatusCode,
		LatencyMS:  latency,
		OK:         httpStatusOK(resp.StatusCode, expect),
	}, nil
}

// httpStatusOK 判定状态码是否符合期望：expect>0 时严格相等，否则按 2xx。
func httpStatusOK(status, expect int) bool {
	if expect > 0 {
		return status == expect
	}
	return status >= 200 && status < 300
}
