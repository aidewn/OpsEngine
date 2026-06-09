// chat 路径的工具循环：模型可在多轮内调用只读工具，再产出最终回复。
//
// 流程：
//  1. 构造 toolSpecs（注册表 → OpenAI tools 格式）
//  2. 非流式 ChatWithTools；若返回 ToolCalls 非空：
//      - 把 assistant tool_calls 消息追加到 messages
//      - 顺序执行每个工具，结果作为 role="tool" 消息追加
//      - 进入下一轮
//  3. 若返回纯文本，作为最终回复
//  4. 防御性上限 MaxToolRounds，超出后报错避免死循环

package runtime

import (
	"encoding/json"
	"fmt"

	"OpsEngine/internal/agent/tools"
	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
)

// defaultMaxToolRounds 是默认工具循环上限。
// 5 轮够覆盖"看清单 → 看进程 → 看日志 → 总结"这种典型链路，又能拦住模型卡死的循环调用。
const defaultMaxToolRounds = 50

// runChatToolLoop 执行带工具的 chat 多轮调用，返回最终文本回复。
// 在循环里通过 emitProgress 把工具调用过程以 🔧 前缀写入进度，让前端无需新事件类型即可展示。
func (r *Runtime) runChatToolLoop(
	req Request,
	session core.AISession,
	messages []clients.ChatMessage,
	progress *[]string,
) (string, error) {
	maxRounds := r.MaxToolRounds
	if maxRounds <= 0 {
		maxRounds = defaultMaxToolRounds
	}
	specs := buildToolSpecs(r.Tools)
	toolCtx := tools.ToolContext{
		SessionID:         session.ID,
		EnvironmentID:     session.EnvironmentID,
		PreferredConfigID: session.ConfigID,
		EnvLookup:         r.Environments,
	}

	for round := 0; round < maxRounds; round++ {
		completion, err := r.LLM.ChatWithTools(messages, specs)
		if err != nil {
			return "", err
		}
		// 没有工具调用 → 当前轮就是最终回复
		if len(completion.ToolCalls) == 0 {
			return completion.Content, nil
		}
		// 把 assistant 的"工具调用消息"追加，方便下一轮模型回看自己说过什么
		messages = append(messages, clients.ChatMessage{
			Role:      "assistant",
			Content:   completion.Content,
			ToolCalls: completion.ToolCalls,
		})
		// 顺序执行所有工具调用并把结果追加为 role="tool" 消息
		for _, call := range completion.ToolCalls {
			toolMsg := r.executeToolCall(req, session.ID, toolCtx, call, progress)
			messages = append(messages, toolMsg)
		}
	}
	return "", fmt.Errorf("Agent 工具循环超过 %d 轮仍未给出最终回复", maxRounds)
}

// executeToolCall 执行单个工具调用并返回要追加到 messages 的 role="tool" 消息。
// 失败时返回 content="<错误描述>" 的 tool 消息，让模型自己读到错误并决定下一步，
// 不直接抛错——单次工具失败不应中断整轮 chat。
func (r *Runtime) executeToolCall(
	req Request,
	sessionID string,
	toolCtx tools.ToolContext,
	call clients.ToolCall,
	progress *[]string,
) clients.ChatMessage {
	name := call.Function.Name
	args := parseToolArgs(call.Function.Arguments)

	// 入参摘要：把 args 拼成一行短描述，便于在 progress 中肉眼读
	r.emitProgress(req.RequestID, sessionID, fmt.Sprintf("🔧 调用 %s%s", name, summarizeArgs(args)), progress)

	tool, ok := r.Tools.Lookup(name)
	if !ok {
		text := fmt.Sprintf("未注册的工具: %s", name)
		r.emitProgress(req.RequestID, sessionID, "✗ "+text, progress)
		return clients.ChatMessage{Role: "tool", ToolCallID: call.ID, Content: text}
	}

	result, err := tool.Execute(toolCtx, args)
	if err != nil {
		text := fmt.Sprintf("%s 执行失败: %s", name, err.Error())
		r.emitProgress(req.RequestID, sessionID, "✗ "+text, progress)
		return clients.ChatMessage{Role: "tool", ToolCallID: call.ID, Content: text}
	}

	summary := result.DisplaySummary
	if summary == "" {
		summary = fmt.Sprintf("%s 完成", name)
	}
	r.emitProgress(req.RequestID, sessionID, "✓ "+summary, progress)
	return clients.ChatMessage{Role: "tool", ToolCallID: call.ID, Content: result.Output}
}

// buildToolSpecs 把 Registry 转成 OpenAI tools 数组。
// 每个工具的 Params 合成 JSON Schema 的 properties + required。
func buildToolSpecs(reg *tools.Registry) []clients.ToolSpec {
	if reg == nil || reg.IsEmpty() {
		return nil
	}
	all := reg.List()
	out := make([]clients.ToolSpec, 0, len(all))
	for _, t := range all {
		spec := t.Spec()
		props := map[string]any{}
		required := []string{}
		for pname, p := range spec.Params {
			entry := map[string]any{
				"type":        p.Type,
				"description": p.Description,
			}
			if p.Default != nil {
				entry["default"] = p.Default
			}
			props[pname] = entry
			if p.Required {
				required = append(required, pname)
			}
		}
		schema := map[string]any{
			"type":       "object",
			"properties": props,
		}
		if len(required) > 0 {
			schema["required"] = required
		}
		out = append(out, clients.ToolSpec{
			Type: "function",
			Function: clients.ToolSpecFunction{
				Name:        spec.Name,
				Description: spec.Description,
				Parameters:  schema,
			},
		})
	}
	return out
}

// parseToolArgs 把 LLM 给的 arguments JSON 字符串解析成 map。
// 失败时返回空 map，让下游工具自己处理"参数缺失"。
func parseToolArgs(raw string) map[string]any {
	out := map[string]any{}
	if raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

// summarizeArgs 把 args 拼成 "(k=v, k=v)" 形式短摘要，过长截断；用于进度文本。
func summarizeArgs(args map[string]any) string {
	if len(args) == 0 {
		return "()"
	}
	parts := []string{}
	for k, v := range args {
		s := fmt.Sprintf("%s=%v", k, v)
		if len(s) > 40 {
			s = s[:37] + "..."
		}
		parts = append(parts, s)
	}
	joined := ""
	for i, p := range parts {
		if i > 0 {
			joined += ", "
		}
		joined += p
	}
	if len(joined) > 80 {
		joined = joined[:77] + "..."
	}
	return "(" + joined + ")"
}
