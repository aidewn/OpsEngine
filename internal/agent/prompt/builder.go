// Prompt Builder：给 Agent Runtime 提供构造 LLM 输入的统一入口。
// 设计目标：调用方只关心"我要生成什么场景的 prompt"，模板加载、变量注入、上下文摘要细节都在本包内部。

package prompt

import (
	"encoding/json"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
)

// WorkflowInputs 是工作流生成 prompt 的注入参数。
// 调用方需先在外层完成节点目录采集 / 环境列表加载，避免 prompt 包反向依赖 store 层。
type WorkflowInputs struct {
	NodeTypes    []core.NodeTypeDef
	Environments []core.EnvironmentDef
	// PreferredEnvironmentID / PreferredConfigID 来自当前会话绑定的 SSH 环境，
	// 模型生成 env_connect_ssh 节点时优先填这两个 id，避免编造。
	PreferredEnvironmentID string
	PreferredConfigID      string
}

// BuildWorkflowSystemPrompt 渲染工作流生成场景的 system prompt。
// 返回完整字符串，调用方直接放进 messages[0].Content。
func BuildWorkflowSystemPrompt(in WorkflowInputs) (string, error) {
	nodeJSON, err := marshalJSON(SummarizeNodeTypes(in.NodeTypes))
	if err != nil {
		return "", err
	}
	envJSON, err := marshalJSON(SummarizeEnvironments(in.Environments))
	if err != nil {
		return "", err
	}
	prefJSON, err := marshalJSON(map[string]string{
		"environment_id": in.PreferredEnvironmentID,
		"ssh_config_id":  in.PreferredConfigID,
	})
	if err != nil {
		return "", err
	}
	return renderTemplate(templateWorkflowGeneration, map[string]string{
		"NodeCatalog":  nodeJSON,
		"Environments": envJSON,
		"Preference":   prefJSON,
	})
}

// InspectionInputs 是巡检计划 prompt 的注入参数。
// 比 WorkflowInputs 窄：模型不需要节点目录，只需要知道目标环境是哪个。
type InspectionInputs struct {
	PreferredEnvironmentID string
	PreferredConfigID      string
}

// BuildArchitectureExplanationPrompt 渲染架构分析的 system prompt。
// summary 是 architecture.RenderTopologySummary 产出的拓扑摘要文本。
func BuildArchitectureExplanationPrompt(summary string) (string, error) {
	return renderTemplate(templateArchitectureExplain, map[string]string{
		"TopologySummary": summary,
	})
}

// BuildInspectionRisksPrompt 渲染巡检报告风险分析 system prompt。
// facts 是 report.RenderFactsForLLM 产出的事实摘要（已剥离冗余日志、控制在合理长度）。
func BuildInspectionRisksPrompt(facts string) (string, error) {
	return renderTemplate(templateInspectionRisks, map[string]string{
		"Facts": facts,
	})
}

// BuildInspectionPlanPrompt 渲染巡检计划生成场景的 system prompt。
// 模型只需输出 JSON 计划，后端自行翻译成 core.WorkflowDef，避免节点 id / 端口编造。
func BuildInspectionPlanPrompt(in InspectionInputs) (string, error) {
	prefJSON, err := marshalJSON(map[string]string{
		"environment_id": in.PreferredEnvironmentID,
		"ssh_config_id":  in.PreferredConfigID,
	})
	if err != nil {
		return "", err
	}
	return renderTemplate(templateInspectionPlan, map[string]string{
		"Preference": prefJSON,
	})
}

// ChatContext 是 BuildChatMessages 的可选注入。
type ChatContext struct {
	// SystemKind 选择 system 提示词；空值默认 SystemPromptChat。
	SystemKind SystemPromptKind
	// Inventory 是预渲染的环境资产清单文本（agentcontext.Inventory.RenderText 的产物）。
	// 空字符串时不注入。
	Inventory string
}

// SystemPromptKind 用来在 BuildChatMessages 中切换 system 提示词的场景。
type SystemPromptKind string

const (
	// SystemPromptChat 是默认的运维助手 system 提示词。
	SystemPromptChat SystemPromptKind = "chat"
	// SystemPromptTroubleshoot 是排障专用 system 提示词，强制"事实/判断/建议"三段输出。
	SystemPromptTroubleshoot SystemPromptKind = "troubleshoot"
)

// systemPromptTemplate 把 SystemPromptKind 映射到 embed 内的模板路径。
func systemPromptTemplate(kind SystemPromptKind) string {
	if kind == SystemPromptTroubleshoot {
		return templateSystemTroubleshoot
	}
	return templateSystemOpsAssistant
}

// BuildChatMessages 把指定的消息序列拼成 LLM 调用的 messages 数组。
// 顺序：ctx.SystemKind 对应的 system 提示词 → Inventory（如有）→ 传入的所有可见消息。
// 空 assistant 消息被丢弃以避免污染上下文（例如用户中断流式回复留下的占位）。
//
// 调用方通常先经过 context.TruncateMessages 裁剪 session.Messages，再传到这里。
func BuildChatMessages(messages []core.AISessionMessage, ctx ChatContext) ([]clients.ChatMessage, error) {
	systemPrompt, err := renderTemplate(systemPromptTemplate(ctx.SystemKind), struct{}{})
	if err != nil {
		return nil, err
	}
	out := []clients.ChatMessage{{Role: "system", Content: systemPrompt}}
	if strings.TrimSpace(ctx.Inventory) != "" {
		out = append(out, clients.ChatMessage{Role: "system", Content: ctx.Inventory})
	}
	for _, m := range messages {
		switch m.Role {
		case core.AIMessageRoleSystem:
			out = append(out, clients.ChatMessage{Role: "system", Content: m.Content})
		case core.AIMessageRoleUser:
			out = append(out, clients.ChatMessage{Role: "user", Content: m.Content})
		case core.AIMessageRoleAssistant:
			if strings.TrimSpace(m.Content) == "" {
				continue
			}
			out = append(out, clients.ChatMessage{Role: "assistant", Content: m.Content})
		}
	}
	return out, nil
}

// marshalJSON 是 json.Marshal 的薄封装，把序列化错误统一加包标记，便于排查。
func marshalJSON(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
