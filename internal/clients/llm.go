// OpenAI 兼容大模型客户端，支持普通和流式 chat completions 调用。
// 默认指向 DeepSeek，但 BaseURL 可换成任何兼容 /chat/completions 的 endpoint。

package clients

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultDeepSeekBaseURL 是默认大模型 API 地址（DeepSeek 官方）。
	DefaultDeepSeekBaseURL = "https://api.deepseek.com"
	// DefaultDeepSeekModel 是默认对话模型名。
	DefaultDeepSeekModel = "deepseek-chat"
)

// LLMClient 封装 OpenAI 兼容 /chat/completions 调用。
type LLMClient struct {
	BaseURL        string
	APIKey         string
	Model          string
	TimeoutSeconds int
}

// ChatMessage 表示一条模型对话消息。
// ToolCalls / ToolCallID 字段支持 OpenAI 兼容的 function calling：
//   - assistant 角色返回工具调用时，ToolCalls 非空
//   - 调用方追加工具结果时，使用 role="tool" + ToolCallID + Content
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall 是模型请求执行的一次工具调用，与 OpenAI 协议字段对齐。
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // 固定 "function"
	Function ToolCallFunc `json:"function"`
}

// ToolCallFunc 是 ToolCall.Function 内容；Arguments 是 JSON 字符串（模型自己序列化）。
type ToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolSpec 是注册给 LLM 的工具元信息（"function calling"协议）。
//   - Parameters 走 JSON Schema 子集，由调用方组装
//   - 一个 ToolSpec 一定 Type="function"
type ToolSpec struct {
	Type     string           `json:"type"`
	Function ToolSpecFunction `json:"function"`
}

// ToolSpecFunction 是 ToolSpec.Function 内容。
type ToolSpecFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ChatCompletion 是 ChatWithTools 的返回：要么有文本，要么有工具调用，可能同时。
type ChatCompletion struct {
	Content   string
	ToolCalls []ToolCall
}

// chatRequest 是 /chat/completions 请求体。
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
	Tools       []ToolSpec    `json:"tools,omitempty"`
	ToolChoice  string        `json:"tool_choice,omitempty"` // "auto" / "none" / "required"
}

// chatResponse 是一次性返回的响应体。
type chatResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
}

// chatStreamResponse 是流式返回的单段响应体。
type chatStreamResponse struct {
	Choices []struct {
		Delta ChatMessage `json:"delta"`
	} `json:"choices"`
}

// Chat 调用一次 /chat/completions，返回第一条候选文本。
func (c LLMClient) Chat(ctx context.Context, messages []ChatMessage) (string, error) {
	if err := c.validate(); err != nil {
		return "", err
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("messages 不能为空")
	}

	body := chatRequest{
		Model:       strings.TrimSpace(c.Model),
		Messages:    messages,
		Temperature: 0.2,
	}
	resp, cleanup, err := c.do(ctx, body)
	if err != nil {
		return "", err
	}
	defer cleanup()

	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", &LLMError{Kind: LLMErrorResponse, Cause: fmt.Errorf("解析大模型响应失败: %w", err)}
	}
	if len(out.Choices) == 0 {
		return "", &LLMError{Kind: LLMErrorResponse, Cause: fmt.Errorf("大模型响应缺少 choices")}
	}
	return out.Choices[0].Message.Content, nil
}

// ChatWithTools 调用 /chat/completions 并允许模型返回工具调用。
// 返回的 ChatCompletion 包含文本（可能为空）和工具调用（可能为空）。
// 调用方据此决定：要么直接采用 Content，要么执行 ToolCalls 后追加结果再调一轮。
//
// 与 Chat 不同：本方法不使用 stream，避免 stream + tool_calls 在不同 provider 表现不一致。
func (c LLMClient) ChatWithTools(ctx context.Context, messages []ChatMessage, tools []ToolSpec) (ChatCompletion, error) {
	if err := c.validate(); err != nil {
		return ChatCompletion{}, err
	}
	if len(messages) == 0 {
		return ChatCompletion{}, fmt.Errorf("messages 不能为空")
	}

	body := chatRequest{
		Model:       strings.TrimSpace(c.Model),
		Messages:    messages,
		Temperature: 0.2,
		Tools:       tools,
	}
	if len(tools) > 0 {
		body.ToolChoice = "auto"
	}
	resp, cleanup, err := c.do(ctx, body)
	if err != nil {
		return ChatCompletion{}, err
	}
	defer cleanup()

	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ChatCompletion{}, &LLMError{Kind: LLMErrorResponse, Cause: fmt.Errorf("解析大模型响应失败: %w", err)}
	}
	if len(out.Choices) == 0 {
		return ChatCompletion{}, &LLMError{Kind: LLMErrorResponse, Cause: fmt.Errorf("大模型响应缺少 choices")}
	}
	m := out.Choices[0].Message
	return ChatCompletion{Content: m.Content, ToolCalls: m.ToolCalls}, nil
}

// ChatStream 调用流式 /chat/completions，每个文本片段通过 onDelta 回调。
func (c LLMClient) ChatStream(ctx context.Context, messages []ChatMessage, onDelta func(string)) (string, error) {
	if err := c.validate(); err != nil {
		return "", err
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("messages 不能为空")
	}

	body := chatRequest{
		Model:       strings.TrimSpace(c.Model),
		Messages:    messages,
		Temperature: 0.2,
		Stream:      true,
	}
	resp, cleanup, err := c.do(ctx, body)
	if err != nil {
		return "", err
	}
	defer cleanup()

	var full strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var out chatStreamResponse
		if err := json.Unmarshal([]byte(payload), &out); err != nil {
			return "", &LLMError{Kind: LLMErrorResponse, Cause: fmt.Errorf("解析大模型流式响应失败: %w", err)}
		}
		for _, choice := range out.Choices {
			if choice.Delta.Content == "" {
				continue
			}
			full.WriteString(choice.Delta.Content)
			if onDelta != nil {
				onDelta(choice.Delta.Content)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", &LLMError{Kind: LLMErrorNetwork, Cause: fmt.Errorf("读取大模型流式响应失败: %w", err)}
	}
	if strings.TrimSpace(full.String()) == "" {
		return "", &LLMError{Kind: LLMErrorResponse, Cause: fmt.Errorf("大模型未返回内容")}
	}
	return full.String(), nil
}

// do 共享 HTTP 请求构造与错误处理，返回响应和 cleanup（关闭 body）。
func (c LLMClient) do(ctx context.Context, body chatRequest) (*http.Response, func(), error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}

	endpoint := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(c.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	timeout := c.TimeoutSeconds
	if timeout <= 0 {
		timeout = 60
	}
	resp, err := (&http.Client{Timeout: time.Duration(timeout) * time.Second}).Do(req)
	if err != nil {
		return nil, nil, classifyTransportError(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, nil, classifyHTTPStatus(resp.StatusCode, resp.Status, string(raw))
	}
	return resp, func() { resp.Body.Close() }, nil
}

// validate 校验 BaseURL/Model 必填。
// APIKey 允许为空以支持本地无鉴权模型。
func (c LLMClient) validate() error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return &LLMError{Kind: LLMErrorConfig, Cause: fmt.Errorf("大模型 base_url 不能为空")}
	}
	if strings.TrimSpace(c.Model) == "" {
		return &LLMError{Kind: LLMErrorConfig, Cause: fmt.Errorf("大模型 model 不能为空")}
	}
	return nil
}

// LLMErrorKind 是 LLM 调用错误的大类，前端据此渲染分类提示。
type LLMErrorKind string

const (
	// LLMErrorConfig 配置错误：base_url / model 缺失、API Key 不合法、401/403 鉴权失败。
	LLMErrorConfig LLMErrorKind = "config"
	// LLMErrorNetwork 网络错误：DNS 解析失败、连接超时、TLS 握手失败、上下文取消等。
	LLMErrorNetwork LLMErrorKind = "network"
	// LLMErrorResponse 模型响应错误：非 2xx 响应体、JSON 解析失败、choices 为空。
	LLMErrorResponse LLMErrorKind = "response"
)

// LLMError 是分类后的 LLM 调用错误。前端用 Kind 决定提示文案，Cause 提供原始原因。
type LLMError struct {
	Kind  LLMErrorKind
	Cause error
}

// Error 输出 "[分类] 原因" 形式的人类可读消息。
func (e *LLMError) Error() string {
	prefix := map[LLMErrorKind]string{
		LLMErrorConfig:   "配置错误",
		LLMErrorNetwork:  "网络错误",
		LLMErrorResponse: "模型响应错误",
	}[e.Kind]
	if prefix == "" {
		prefix = string(e.Kind)
	}
	return fmt.Sprintf("[%s] %s", prefix, e.Cause.Error())
}

// Unwrap 暴露原因错误，供 errors.Is / errors.As 使用。
func (e *LLMError) Unwrap() error { return e.Cause }

// classifyTransportError 把 http.Client.Do 返回的 transport 错误分类为网络错误，
// 并对超时 / context 取消给出更明确的提示。
func classifyTransportError(err error) error {
	msg := strings.ToLower(err.Error())
	hint := err.Error()
	switch {
	case strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "timeout"):
		hint = "请求超时，请检查网络或调大超时时间：" + hint
	case strings.Contains(msg, "no such host") || strings.Contains(msg, "dns"):
		hint = "无法解析主机，请检查 base_url：" + hint
	case strings.Contains(msg, "refused") || strings.Contains(msg, "connect"):
		hint = "无法连接到模型服务：" + hint
	}
	return &LLMError{Kind: LLMErrorNetwork, Cause: fmt.Errorf("%s", hint)}
}

// classifyHTTPStatus 根据 HTTP 状态码区分鉴权配置错误与模型响应错误。
func classifyHTTPStatus(status int, statusText, body string) error {
	suffix := strings.TrimSpace(body)
	if status == 401 || status == 403 {
		return &LLMError{
			Kind:  LLMErrorConfig,
			Cause: fmt.Errorf("鉴权失败 (%s)，请检查 API Key：%s", statusText, suffix),
		}
	}
	return &LLMError{
		Kind:  LLMErrorResponse,
		Cause: fmt.Errorf("大模型请求失败: %s %s", statusText, suffix),
	}
}
