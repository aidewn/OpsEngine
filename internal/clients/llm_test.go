// LLM 客户端错误分类测试：覆盖配置 / 网络 / 模型响应三类错误的边界。

package clients

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestValidateReturnsConfigError 验证 BaseURL/Model 缺失时归为配置错误。
func TestValidateReturnsConfigError(t *testing.T) {
	_, err := LLMClient{}.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected error")
	}
	var llmErr *LLMError
	if !errors.As(err, &llmErr) || llmErr.Kind != LLMErrorConfig {
		t.Fatalf("expected LLMErrorConfig, got %v", err)
	}
}

// Test401IsConfigError 验证 401/403 归到配置错误而不是网络。
func Test401IsConfigError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()
	c := LLMClient{BaseURL: srv.URL, Model: "x", APIKey: "k"}
	_, err := c.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}})
	var llmErr *LLMError
	if !errors.As(err, &llmErr) || llmErr.Kind != LLMErrorConfig {
		t.Fatalf("expected LLMErrorConfig, got %v", err)
	}
	if !strings.Contains(llmErr.Error(), "鉴权失败") {
		t.Fatalf("expected 鉴权失败 hint, got %s", llmErr.Error())
	}
}

// Test500IsResponseError 验证 5xx 归到模型响应错误。
func Test500IsResponseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	c := LLMClient{BaseURL: srv.URL, Model: "x"}
	_, err := c.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}})
	var llmErr *LLMError
	if !errors.As(err, &llmErr) || llmErr.Kind != LLMErrorResponse {
		t.Fatalf("expected LLMErrorResponse, got %v", err)
	}
}

// TestInvalidJSONIsResponseError 验证 2xx 但返回非 JSON 时归为模型响应错误。
func TestInvalidJSONIsResponseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	c := LLMClient{BaseURL: srv.URL, Model: "x"}
	_, err := c.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}})
	var llmErr *LLMError
	if !errors.As(err, &llmErr) || llmErr.Kind != LLMErrorResponse {
		t.Fatalf("expected LLMErrorResponse, got %v", err)
	}
}

// TestSlowResponseTimeoutIsNetworkError 验证响应读取阶段的超时不再被误判为"解析失败"。
// 模拟：server 接受连接、返回 200，但故意延迟 1.5s 写响应体，客户端 timeout 1s。
func TestSlowResponseTimeoutIsNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		// flush header 后阻塞，让客户端 timeout 在读 body 时触发
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		time.Sleep(1500 * time.Millisecond)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi"}}]}`))
	}))
	defer srv.Close()
	c := LLMClient{BaseURL: srv.URL, Model: "x", TimeoutSeconds: 1}
	_, err := c.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}})
	var llmErr *LLMError
	if !errors.As(err, &llmErr) {
		t.Fatalf("expected LLMError, got %v", err)
	}
	if llmErr.Kind != LLMErrorNetwork {
		t.Fatalf("expected LLMErrorNetwork (timeout in read), got Kind=%s msg=%s", llmErr.Kind, llmErr.Error())
	}
	if !strings.Contains(llmErr.Error(), "请求超时") {
		t.Fatalf("expected 请求超时 hint, got %s", llmErr.Error())
	}
}

// TestUnreachableHostIsNetworkError 验证连接失败归为网络错误。
func TestUnreachableHostIsNetworkError(t *testing.T) {
	// 用一个肯定不通的端口（保留段 127.0.0.0/8 上随机端口），保持快速失败。
	c := LLMClient{BaseURL: "http://127.0.0.1:1", Model: "x", TimeoutSeconds: 2}
	_, err := c.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}})
	var llmErr *LLMError
	if !errors.As(err, &llmErr) || llmErr.Kind != LLMErrorNetwork {
		t.Fatalf("expected LLMErrorNetwork, got %v", err)
	}
}
