// OpenAI 兼容大模型客户端，支持普通和流式 chat completions 调用。
// 默认指向 DeepSeek，但 BaseURL 可换成任何兼容 /chat/completions 的 endpoint。

package clients

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
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
		return "", classifyReadError(err)
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
		return ChatCompletion{}, classifyReadError(err)
	}
	if len(out.Choices) == 0 {
		return ChatCompletion{}, &LLMError{Kind: LLMErrorResponse, Cause: fmt.Errorf("大模型响应缺少 choices")}
	}
	m := out.Choices[0].Message
	return ChatCompletion{Content: m.Content, ToolCalls: m.ToolCalls}, nil
}

// streamToolCallDelta 是 OpenAI 流式协议中 tool_calls 的单段增量。
// 同一个 tool call 会跨多个 chunk：第一段带 id + function.name，后续段只追加 function.arguments。
// Index 字段是 provider 给的序号，跨段稳定，调用方据此累积。
type streamToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

// streamChunkWithTools 是支持 tool_calls 的流式响应单段。
type streamChunkWithTools struct {
	Choices []struct {
		Delta struct {
			Content   string                `json:"content,omitempty"`
			ToolCalls []streamToolCallDelta `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
}

// ChatWithToolsStream 是 ChatWithTools 的流式版本：content 通过 onContent 实时回调，
// 同时累积 tool_calls 等流式结束后一并返回。
//
// 适用场景：聊天工具循环里，希望让用户在 LLM 推理阶段就能看到文本，
// 避免"几十秒空转 + 文本一次性 dump"的体验。
//
// 与 ChatStream 不同：
//   - 支持 tools 参数
//   - 累积 tool_calls，最终一起返回（调用方据此决定是执行工具还是把 content 当作终态）
//   - 内部走 SSE，sleep/timeout 与 ChatStream 一致
func (c LLMClient) ChatWithToolsStream(
	ctx context.Context,
	messages []ChatMessage,
	tools []ToolSpec,
	onContent func(string),
) (ChatCompletion, error) {
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
		Stream:      true,
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

	var contentBuf strings.Builder
	// 按 Index 累积 tool_calls；最终按 Index 升序输出，与 OpenAI 协议契约一致。
	toolCallsByIdx := map[int]*ToolCall{}

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
		var chunk streamChunkWithTools
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return ChatCompletion{}, &LLMError{Kind: LLMErrorResponse, Cause: fmt.Errorf("解析大模型流式响应失败: %w", err)}
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				contentBuf.WriteString(choice.Delta.Content)
				if onContent != nil {
					onContent(choice.Delta.Content)
				}
			}
			for _, tc := range choice.Delta.ToolCalls {
				existing, ok := toolCallsByIdx[tc.Index]
				if !ok {
					existing = &ToolCall{Type: "function"}
					toolCallsByIdx[tc.Index] = existing
				}
				if tc.ID != "" {
					existing.ID = tc.ID
				}
				if tc.Type != "" {
					existing.Type = tc.Type
				}
				if tc.Function.Name != "" {
					existing.Function.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					// arguments 是分段拼接的 JSON 字符串；继续追加，最终由调用方 json.Unmarshal
					existing.Function.Arguments += tc.Function.Arguments
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatCompletion{}, classifyReadError(err)
	}

	// 按 Index 升序输出，行为与非流式 ChatWithTools 等价
	indices := make([]int, 0, len(toolCallsByIdx))
	for k := range toolCallsByIdx {
		indices = append(indices, k)
	}
	sort.Ints(indices)
	toolCalls := make([]ToolCall, 0, len(indices))
	for _, idx := range indices {
		toolCalls = append(toolCalls, *toolCallsByIdx[idx])
	}

	return ChatCompletion{Content: contentBuf.String(), ToolCalls: toolCalls}, nil
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
			// 流式分段解析失败基本只可能是 provider 真的返回了非法 JSON 行（与超时无关）。
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
		return "", classifyReadError(err)
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
		// 默认 120s：生成集合 / 复杂工作流的大模型回复经常 60s 不够。
		// 简单的 chat 请求实际响应远低于此值，不会有体感差异。
		timeout = 120
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
		hint = "请求超时，请检查网络或在 AI 设置中调大超时时间：" + hint
	case strings.Contains(msg, "no such host") || strings.Contains(msg, "dns"):
		hint = "无法解析主机，请检查 base_url：" + hint
	case strings.Contains(msg, "refused") || strings.Contains(msg, "connect"):
		hint = "无法连接到模型服务：" + hint
	}
	return &LLMError{Kind: LLMErrorNetwork, Cause: fmt.Errorf("%s", hint)}
}

// classifyReadError 区分"读取响应体时超时"与"真的解析失败"。
//
// 关键点：http.Client.Timeout 在等首字节之外也覆盖整个响应读取，
// 所以请求成功后再读 body 也可能因为模型生成太慢而 deadline exceeded。
// 这种场景以前被错误标成"模型响应错误 / 解析失败"，让用户搞不清是网络还是真的吐了乱码。
func classifyReadError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &LLMError{Kind: LLMErrorNetwork, Cause: fmt.Errorf("请求超时（响应读取阶段），请在 AI 设置中调大超时时间或简化请求")}
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "timeout") || strings.Contains(msg, "i/o timeout") {
		return &LLMError{Kind: LLMErrorNetwork, Cause: fmt.Errorf("请求超时（响应读取阶段），请在 AI 设置中调大超时时间或简化请求")}
	}
	return &LLMError{Kind: LLMErrorResponse, Cause: fmt.Errorf("解析大模型响应失败: %w", err)}
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
