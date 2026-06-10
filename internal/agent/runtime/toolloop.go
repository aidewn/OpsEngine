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
	"sync/atomic"
	"time"

	"OpsEngine/internal/agent/tools"
	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
)

// llmHeartbeatInterval 是 LLM 调用期间心跳事件的间隔。
// 选 2s：足够快让用户看到"还活着"，又不至于刷屏（前端单行原地刷新）。
const llmHeartbeatInterval = 2 * time.Second

// defaultMaxToolRounds 是默认工具循环上限。
// 50 轮够覆盖"看清单 → 看进程 → 看日志 → 总结"这种典型链路，又能拦住模型卡死的循环调用。
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
	toolCtx := buildToolContext(r, session)

	for round := 0; round < maxRounds; round++ {
		// 每轮调用前推一条心跳进度，避免长时间无反馈让用户以为卡住
		if round == 0 {
			r.emitProgress(req.RequestID, session.ID, "正在思考…", progress)
		} else {
			r.emitProgress(req.RequestID, session.ID, "正在基于工具结果继续推理…", progress)
		}

		// 启动心跳 goroutine：DeepSeek 在 tools 模式下经常不真正流 content，
		// 思考阶段会有几十秒"假死"，靠这里的 ticker 推 heartbeat 让 UI 显示已用时长。
		// 一旦收到任意 content delta 就把心跳静音（gotDelta=true），避免心跳与流式文本互相覆盖。
		stopHeartbeat := make(chan struct{})
		startedAt := time.Now()
		var gotDelta atomic.Bool
		go func() {
			ticker := time.NewTicker(llmHeartbeatInterval)
			defer ticker.Stop()
			for {
				select {
				case <-stopHeartbeat:
					return
				case <-ticker.C:
					if gotDelta.Load() {
						continue
					}
					r.emitHeartbeat(req.RequestID, session.ID,
						fmt.Sprintf("思考中（%ds）", int(time.Since(startedAt).Seconds())))
				}
			}
		}()

		// 走流式：content 一边出一边推送给前端；LLM 网络类错误自动重试
		var completion clients.ChatCompletion
		var err error
		for attempt := 0; attempt < defaultArtifactGenRetries; attempt++ {
			if attempt > 0 {
				r.emitProgress(req.RequestID, session.ID, "模型调用失败，正在重试…", progress)
			}
			completion, err = r.LLM.ChatWithToolsStream(messages, specs, func(delta string) {
				if delta == "" || r.Emit == nil {
					return
				}
				gotDelta.Store(true)
				r.Emit.Emit(Event{
					RequestID: req.RequestID, SessionID: session.ID,
					Type: EventDelta, Text: delta,
				})
			})
			if err == nil {
				break
			}
			if attempt >= defaultArtifactGenRetries-1 || !isRetryableLLMError(err) {
				break
			}
		}
		close(stopHeartbeat)
		if err != nil {
			return "", err
		}
		// 没有工具调用 → 当前轮就是最终回复（content 已经流式推完）
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

// buildToolContext 组装工具执行上下文，合并 Registry 注入与 Runtime 回调。
func buildToolContext(r *Runtime, session core.AISession) tools.ToolContext {
	ctx := tools.ToolContext{
		SessionID:         session.ID,
		EnvironmentID:     session.EnvironmentID,
		PreferredConfigID: session.ConfigID,
		EnvLookup:         r.Environments,
	}
	if r.Tools != nil {
		deps := r.Tools.Deps()
		ctx.NodeCatalog = deps.NodeCatalog
		ctx.WorkflowGet = deps.WorkflowGet
		ctx.AssembleGet = deps.AssembleGet
		ctx.WorkflowList = deps.WorkflowList
		ctx.AssembleList = deps.AssembleList
	}
	if ctx.NodeCatalog == nil && r.Nodes != nil {
		ctx.NodeCatalog = r.Nodes
	}
	if ctx.WorkflowGet == nil && r.Workflows != nil {
		ctx.WorkflowGet = r.Workflows.Get
	}
	if ctx.AssembleGet == nil && r.Assembles != nil {
		ctx.AssembleGet = r.Assembles.Get
	}
	return ctx
}
