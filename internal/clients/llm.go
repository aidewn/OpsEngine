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
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest 是 /chat/completions 请求体。
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
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
		return "", fmt.Errorf("解析大模型响应失败: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("大模型响应缺少 choices")
	}
	return out.Choices[0].Message.Content, nil
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
			return "", fmt.Errorf("解析大模型流式响应失败: %w", err)
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
		return "", fmt.Errorf("读取大模型流式响应失败: %w", err)
	}
	if strings.TrimSpace(full.String()) == "" {
		return "", fmt.Errorf("大模型未返回内容")
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
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, nil, fmt.Errorf("大模型请求失败: %s %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	return resp, func() { resp.Body.Close() }, nil
}

// validate 校验 BaseURL/Model 必填。
// APIKey 允许为空以支持本地无鉴权模型。
func (c LLMClient) validate() error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("大模型 base_url 不能为空")
	}
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("大模型 model 不能为空")
	}
	return nil
}
