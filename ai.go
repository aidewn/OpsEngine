// AI 设置与统一助手入口，负责会话管理、SSH 上下文预取、大模型对话与工作流生成。

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	"github.com/BurntSushi/toml"
	"github.com/google/uuid"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// AISettingsFile 是本地 AI 配置文件路径。
const AISettingsFile = "data/settings/ai.toml"

// aiPrefetchTimeout 是 SSH 服务器信息采集的硬超时。
const aiPrefetchTimeout = 20 * time.Second

// AISettings 保存外部大模型连接参数。字段名沿用 deepseek_* 以兼容已有配置文件，
// 但底层走的是 OpenAI 兼容协议，base_url 换成任何兼容端点都能用。
type AISettings struct {
	DeepSeekAPIKey  string `json:"deepseek_api_key"  toml:"deepseek_api_key"`
	DeepSeekBaseURL string `json:"deepseek_base_url" toml:"deepseek_base_url"`
	DeepSeekModel   string `json:"deepseek_model"    toml:"deepseek_model"`
	TimeoutSeconds  int    `json:"timeout_seconds"   toml:"timeout_seconds"`
}

// AIAssistantRequest 是统一 AI 助手请求。
type AIAssistantRequest struct {
	// RequestID 是前端生成的请求 ID，用于事件归属。
	RequestID string `json:"request_id"`
	// SessionID 指向已有会话；StartAIAssistant 前必须先 CreateAISession 拿到。
	SessionID string `json:"session_id"`
	// Operation 兼容字段；传 auto 或留空时由后端根据 Message 自动判断。
	Operation string `json:"operation"`
	// Message 是用户输入。
	Message string `json:"message"`
}

// AIAssistantEvent 是后端推送给前端的 AI 助手事件。
type AIAssistantEvent struct {
	RequestID    string `json:"request_id"`
	SessionID    string `json:"session_id,omitempty"`
	Type         string `json:"type"`
	Text         string `json:"text,omitempty"`
	WorkflowID   string `json:"workflow_id,omitempty"`
	WorkflowName string `json:"workflow_name,omitempty"`
}

// aiGeneratedNode 是模型输出的临时节点（id 为临时占位，后端会替换为 UUID）。
type aiGeneratedNode struct {
	ID       string         `json:"id"`
	TypeID   string         `json:"type_id"`
	Config   map[string]any `json:"config"`
	Position aiPosition     `json:"position"`
}

// aiPosition 是模型输出的画布坐标。
type aiPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// aiPortRef 是模型输出的端口引用。
type aiPortRef struct {
	Node string `json:"node"`
	Port string `json:"port"`
}

// aiGeneratedEdge 是模型输出的临时边。
type aiGeneratedEdge struct {
	From aiPortRef `json:"from"`
	To   aiPortRef `json:"to"`
}

// aiGeneratedWorkflow 是模型输出的工作流草案。
type aiGeneratedWorkflow struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Variables   []core.VariableDef `json:"variables"`
	Nodes       []aiGeneratedNode  `json:"nodes"`
	Edges       []aiGeneratedEdge  `json:"edges"`
	Notes       []string           `json:"notes"`
}

// ── 设置 CRUD ───────────────────────────────────────────────

// GetAISettings 读取 AI 设置，缺失配置文件时返回默认值。
func (a *App) GetAISettings() (AISettings, error) {
	return loadAISettings()
}

// UpdateAISettings 保存 AI 设置到 data/settings/ai.toml。
func (a *App) UpdateAISettings(settings AISettings) error {
	settings = normalizeAISettings(settings)
	if strings.TrimSpace(settings.DeepSeekAPIKey) == "" {
		return errors.New("API Key 不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(AISettingsFile), 0755); err != nil {
		return fmt.Errorf("创建 AI 配置目录失败: %w", err)
	}
	file, err := os.Create(AISettingsFile)
	if err != nil {
		return fmt.Errorf("保存 AI 配置失败: %w", err)
	}
	defer file.Close()
	return toml.NewEncoder(file).Encode(settings)
}

// TestAISettings 使用已保存配置发送一次轻量请求，验证 API 是否可用。
func (a *App) TestAISettings() (string, error) {
	settings, err := loadAISettings()
	if err != nil {
		return "", err
	}
	return a.callLLM(settings, []clients.ChatMessage{
		{Role: "system", Content: "你只需要用中文回复\"连接成功\"四个字。"},
		{Role: "user", Content: "请确认连接状态。"},
	})
}

// ── 会话 CRUD ───────────────────────────────────────────────

// ListAISessions 列出所有 AI 会话（按 UpdatedAt 倒序）。
func (a *App) ListAISessions() ([]core.AISession, error) {
	if a.aiSessionStore == nil {
		return []core.AISession{}, nil
	}
	return a.aiSessionStore.List()
}

// GetAISession 按 ID 加载会话。
func (a *App) GetAISession(id string) (core.AISession, error) {
	if a.aiSessionStore == nil {
		return core.AISession{}, errors.New("会话存储未初始化")
	}
	return a.aiSessionStore.Get(id)
}

// CreateAISession 创建空会话并返回。EnvironmentID/ConfigID 必须是有效的 SSH 配置。
// Title 留空时使用占位标题，前端可以稍后通过 UpdateAISessionTitle 改写。
func (a *App) CreateAISession(environmentID, configID, title string) (core.AISession, error) {
	if err := a.validateAIEnvironment(environmentID, configID); err != nil {
		return core.AISession{}, err
	}
	if a.aiSessionStore == nil {
		return core.AISession{}, errors.New("会话存储未初始化")
	}
	now := time.Now()
	title = strings.TrimSpace(title)
	if title == "" {
		title = "AI 助手会话 " + now.Format("01-02 15:04")
	}
	session := core.AISession{
		ID:            uuid.New().String(),
		Title:         title,
		EnvironmentID: environmentID,
		ConfigID:      configID,
		Messages:      []core.AISessionMessage{},
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := a.aiSessionStore.Save(session); err != nil {
		return core.AISession{}, err
	}
	return session, nil
}

// UpdateAISessionTitle 重命名会话。
func (a *App) UpdateAISessionTitle(id, title string) error {
	if a.aiSessionStore == nil {
		return errors.New("会话存储未初始化")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("会话标题不能为空")
	}
	session, err := a.aiSessionStore.Get(id)
	if err != nil {
		return err
	}
	session.Title = title
	session.UpdatedAt = time.Now()
	return a.aiSessionStore.Save(session)
}

// DeleteAISession 删除会话。
func (a *App) DeleteAISession(id string) error {
	if a.aiSessionStore == nil {
		return errors.New("会话存储未初始化")
	}
	return a.aiSessionStore.Delete(id)
}

// ── 统一助手入口 ────────────────────────────────────────────

// StartAIAssistant 把用户消息追加到会话并启动 chat 或 generate_workflow 流程。
func (a *App) StartAIAssistant(req AIAssistantRequest) error {
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.Operation = resolveAIAssistantOperation(req.Operation, req.Message)
	req.Message = strings.TrimSpace(req.Message)

	if req.RequestID == "" {
		return errors.New("request_id 不能为空")
	}
	if req.SessionID == "" {
		a.emitAIAssistant(req.RequestID, "", "error", "session_id 不能为空")
		return nil
	}
	if req.Message == "" {
		a.emitAIAssistant(req.RequestID, req.SessionID, "error", "请输入要发送给 AI 的内容")
		return nil
	}
	if a.aiSessionStore == nil {
		a.emitAIAssistant(req.RequestID, req.SessionID, "error", "会话存储未初始化")
		return nil
	}

	session, err := a.aiSessionStore.Get(req.SessionID)
	if err != nil {
		a.emitAIAssistant(req.RequestID, req.SessionID, "error", err.Error())
		return nil
	}
	if err := a.validateAIEnvironment(session.EnvironmentID, session.ConfigID); err != nil {
		a.emitAIAssistant(req.RequestID, session.ID, "error", err.Error())
		return nil
	}
	settings, err := loadAISettings()
	if err != nil {
		a.emitAIAssistant(req.RequestID, session.ID, "error", err.Error())
		return nil
	}

	now := time.Now()
	userMsg := core.AISessionMessage{
		ID:        uuid.New().String(),
		Role:      core.AIMessageRoleUser,
		Content:   req.Message,
		CreatedAt: now,
	}
	session.Messages = append(session.Messages, userMsg)
	// 首条用户消息时自动用 message 前 30 字符作为标题，便于在侧边栏识别。
	if isFirstUserMessage(session) {
		session.Title = makeSessionTitle(req.Message)
	}
	session.UpdatedAt = now
	if err := a.aiSessionStore.Save(session); err != nil {
		a.emitAIAssistant(req.RequestID, session.ID, "error", err.Error())
		return nil
	}

	switch req.Operation {
	case "generate_workflow":
		a.startAIWorkflowGeneration(req, session, settings)
	default:
		a.startAIChatStream(req, session, settings)
	}
	return nil
}

// ── 内部：chat 与 generate_workflow ──────────────────────────

// startAIChatStream 在必要时先采集 SSH 服务器信息，再以流式方式回复用户。
func (a *App) startAIChatStream(req AIAssistantRequest, session core.AISession, settings AISettings) {
	progress := []string{}
	if !session.ContextPrefetched {
		a.emitAIAssistant(req.RequestID, session.ID, "progress", "正在采集服务器信息")
		progress = append(progress, "正在采集服务器信息")
		contextText, err := a.prefetchSSHContext(session.EnvironmentID, session.ConfigID)
		if err != nil {
			// 采集失败不致命，继续走 LLM，但告知用户。
			notice := "服务器信息采集失败：" + err.Error()
			a.emitAIAssistant(req.RequestID, session.ID, "progress", notice)
			progress = append(progress, notice)
		} else {
			session.Messages = append(session.Messages, core.AISessionMessage{
				ID:        uuid.New().String(),
				Role:      core.AIMessageRoleSystem,
				Content:   contextText,
				Hidden:    true,
				CreatedAt: time.Now(),
			})
			session.ContextPrefetched = true
			if err := a.aiSessionStore.Save(session); err != nil {
				a.emitAIAssistant(req.RequestID, session.ID, "error", err.Error())
				return
			}
		}
	}

	messages := buildChatLLMMessages(session)
	assistantContent := strings.Builder{}
	_, err := a.callLLMStream(settings, messages, func(delta string) {
		assistantContent.WriteString(delta)
		a.emitAIAssistant(req.RequestID, session.ID, "delta", delta)
	})
	if err != nil {
		a.emitAIAssistant(req.RequestID, session.ID, "error", err.Error())
		return
	}

	assistantMsg := core.AISessionMessage{
		ID:        uuid.New().String(),
		Role:      core.AIMessageRoleAssistant,
		Content:   assistantContent.String(),
		Progress:  progress,
		CreatedAt: time.Now(),
	}
	session.Messages = append(session.Messages, assistantMsg)
	session.UpdatedAt = time.Now()
	if err := a.aiSessionStore.Save(session); err != nil {
		a.emitAIAssistant(req.RequestID, session.ID, "error", err.Error())
		return
	}
	a.emitAIAssistant(req.RequestID, session.ID, "done", "")
}

// startAIWorkflowGeneration 让大模型基于节点目录和环境列表生成任意工作流。
func (a *App) startAIWorkflowGeneration(req AIAssistantRequest, session core.AISession, settings AISettings) {
	progress := []string{}
	emitProgress := func(text string) {
		progress = append(progress, text)
		a.emitAIAssistant(req.RequestID, session.ID, "progress", text)
	}

	emitProgress("正在收集节点目录与环境信息")
	systemPrompt := a.buildWorkflowSystemPrompt(session)

	emitProgress("正在请求大模型生成工作流")
	reply, err := a.callLLM(settings, []clients.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: req.Message},
	})
	if err != nil {
		a.emitAIAssistant(req.RequestID, session.ID, "error", err.Error())
		return
	}

	emitProgress("正在解析模型返回")
	draft, err := parseGeneratedWorkflow(reply)
	if err != nil {
		a.emitAIAssistant(req.RequestID, session.ID, "error", err.Error())
		return
	}

	emitProgress("正在校验工作流结构")
	workflow, err := a.materializeWorkflow(draft)
	if err != nil {
		a.emitAIAssistant(req.RequestID, session.ID, "error", err.Error())
		return
	}

	emitProgress("正在保存工作流")
	if a.workflowStore == nil {
		a.emitAIAssistant(req.RequestID, session.ID, "error", "工作流存储未初始化")
		return
	}
	if err := a.workflowStore.Save(workflow); err != nil {
		a.emitAIAssistant(req.RequestID, session.ID, "error", err.Error())
		return
	}

	wailsruntime.EventsEmit(a.ctx, "ai:assistant", AIAssistantEvent{
		RequestID:    req.RequestID,
		SessionID:    session.ID,
		Type:         "workflow",
		WorkflowID:   workflow.ID,
		WorkflowName: workflow.Name,
	})

	session.Messages = append(session.Messages, core.AISessionMessage{
		ID:           uuid.New().String(),
		Role:         core.AIMessageRoleAssistant,
		Content:      fmt.Sprintf("已生成工作流「%s」，可以直接打开查看。", workflow.Name),
		Progress:     progress,
		WorkflowID:   workflow.ID,
		WorkflowName: workflow.Name,
		CreatedAt:    time.Now(),
	})
	session.UpdatedAt = time.Now()
	if err := a.aiSessionStore.Save(session); err != nil {
		a.emitAIAssistant(req.RequestID, session.ID, "error", err.Error())
		return
	}
	a.emitAIAssistant(req.RequestID, session.ID, "done", "")
}

// ── 内部：LLM 调用封装 ──────────────────────────────────────

// callLLM 一次性调用大模型。
func (a *App) callLLM(settings AISettings, messages []clients.ChatMessage) (string, error) {
	client, ctx, cancel := a.newLLMCall(settings)
	defer cancel()
	return client.Chat(ctx, messages)
}

// callLLMStream 流式调用大模型，每个增量通过 onDelta 回调。
func (a *App) callLLMStream(settings AISettings, messages []clients.ChatMessage, onDelta func(string)) (string, error) {
	client, ctx, cancel := a.newLLMCall(settings)
	defer cancel()
	return client.ChatStream(ctx, messages, onDelta)
}

// newLLMCall 根据 settings 构造客户端和带超时的 context。
func (a *App) newLLMCall(settings AISettings) (clients.LLMClient, context.Context, context.CancelFunc) {
	settings = normalizeAISettings(settings)
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	timeout := time.Duration(settings.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	client := clients.LLMClient{
		BaseURL:        settings.DeepSeekBaseURL,
		APIKey:         settings.DeepSeekAPIKey,
		Model:          settings.DeepSeekModel,
		TimeoutSeconds: settings.TimeoutSeconds,
	}
	return client, ctx, cancel
}

// ── 内部：SSH 上下文预取 ────────────────────────────────────

// prefetchSSHContext 通过 SSH 执行一组只读巡检命令，把结果格式化成 Markdown，
// 注入到会话作为 system role 消息让 LLM 后续围绕真实数据回答。
func (a *App) prefetchSSHContext(environmentID, configID string) (string, error) {
	env, err := a.environmentStore.Get(environmentID)
	if err != nil {
		return "", err
	}
	var fields map[string]any
	for _, c := range env.Configs {
		if c.ID == configID {
			fields = c.Fields
			break
		}
	}
	if fields == nil {
		return "", fmt.Errorf("环境 %s 中未找到配置 %s", environmentID, configID)
	}
	dial, err := clients.ParseLinuxSshDialConfig(fields)
	if err != nil {
		return "", err
	}
	client, err := dial.Dial()
	if err != nil {
		return "", err
	}
	defer client.Close()

	session, err := client.Client().NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := session.CombinedOutput(prefetchScript)
		done <- result{out, err}
	}()

	select {
	case r := <-done:
		text := truncateOutput(string(r.out), 12000)
		header := fmt.Sprintf("以下是目标服务器（环境 %s / 配置 %s）当前的真实状态，回答用户问题时请优先参考这些数据：\n\n", env.Name, configID)
		// 命令非零退出不致命，仍把 stdout 暴露给模型。
		return header + "```\n" + text + "\n```", nil
	case <-time.After(aiPrefetchTimeout):
		_ = session.Close()
		return "", fmt.Errorf("采集超时（%s）", aiPrefetchTimeout)
	}
}

// prefetchScript 是只读巡检脚本，覆盖 OS / 负载 / 内存 / 磁盘 / 进程 / 网络。
// 每行都加 2>/dev/null 兼容缺工具的最小镜像。
const prefetchScript = `
set +e
echo "## 系统信息"
uname -a 2>/dev/null
(cat /etc/os-release 2>/dev/null || cat /etc/issue 2>/dev/null) | head -10
echo
echo "## 运行时长与负载"
uptime 2>/dev/null
echo
echo "## CPU"
(lscpu 2>/dev/null | head -15) || cat /proc/cpuinfo 2>/dev/null | head -10
echo
echo "## 内存"
(free -h 2>/dev/null) || (head -5 /proc/meminfo 2>/dev/null)
echo
echo "## 磁盘"
df -hT 2>/dev/null
echo
echo "## CPU 占用前 10 进程"
ps -eo pid,user,pcpu,pmem,comm --sort=-pcpu 2>/dev/null | head -11
echo
echo "## 网卡地址"
(ip -br addr 2>/dev/null) || (ifconfig 2>/dev/null | head -20)
echo
echo "## 监听端口"
(ss -tln 2>/dev/null | head -20) || (netstat -tln 2>/dev/null | head -20)
`

// truncateOutput 限制单次注入 prompt 的字节数，避免吃满上下文窗口。
func truncateOutput(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	return text[:maxBytes] + "\n... (输出已截断)"
}

// ── 内部：事件、意图、工作流生成 ───────────────────────────

// emitAIAssistant 向前端发送一条 AI 助手事件。
func (a *App) emitAIAssistant(requestID, sessionID, eventType, text string) {
	if a.ctx == nil {
		return
	}
	wailsruntime.EventsEmit(a.ctx, "ai:assistant", AIAssistantEvent{
		RequestID: requestID,
		SessionID: sessionID,
		Type:      eventType,
		Text:      text,
	})
}

// validateAIEnvironment 校验所选环境与 SSH 配置。
func (a *App) validateAIEnvironment(environmentID, configID string) error {
	environmentID = strings.TrimSpace(environmentID)
	configID = strings.TrimSpace(configID)
	if environmentID == "" || configID == "" {
		return errors.New("请先配置环境，并选择环境中的 SSH 配置")
	}
	if a.environmentStore == nil {
		return errors.New("环境存储未初始化")
	}
	env, err := a.environmentStore.Get(environmentID)
	if err != nil {
		return err
	}
	for _, config := range env.Configs {
		if config.ID == configID {
			if config.Kind != core.EnvConfigKindSSH {
				return fmt.Errorf("配置 %s 不是 SSH 配置", configID)
			}
			return nil
		}
	}
	return fmt.Errorf("环境 %s 中未找到 SSH 配置 %s", environmentID, configID)
}

// resolveAIAssistantOperation 通过关键词识别用户意图。
// 命中工作流相关词 → generate_workflow；否则走 chat。
func resolveAIAssistantOperation(operation, message string) string {
	operation = strings.TrimSpace(operation)
	if operation != "" && operation != "auto" {
		return operation
	}
	text := strings.ToLower(strings.TrimSpace(message))
	for _, keyword := range []string{
		"工作流", "巡检", "流程",
		"workflow", "inspection", "pipeline",
	} {
		if strings.Contains(text, keyword) {
			return "generate_workflow"
		}
	}
	return "chat"
}

// buildChatLLMMessages 把会话历史转换为 LLM 调用的 messages 数组。
// 顺序：基础 system 提示 → 历史中所有可见消息 + 预取的 system 消息（hidden）。
func buildChatLLMMessages(session core.AISession) []clients.ChatMessage {
	messages := []clients.ChatMessage{
		{
			Role: "system",
			Content: "你是 OpsEngine 的运维助手。用户已选择目标 SSH 环境。" +
				"如果会话上文中已提供\"目标服务器当前状态\"的数据块，请把它视为真实事实，并基于其中的数值给出具体、可执行的诊断和建议；" +
				"不要笼统回答，也不要让用户自己再去跑命令收集这些信息。",
		},
	}
	for _, m := range session.Messages {
		switch m.Role {
		case core.AIMessageRoleSystem:
			messages = append(messages, clients.ChatMessage{Role: "system", Content: m.Content})
		case core.AIMessageRoleUser:
			messages = append(messages, clients.ChatMessage{Role: "user", Content: m.Content})
		case core.AIMessageRoleAssistant:
			if strings.TrimSpace(m.Content) == "" {
				continue
			}
			messages = append(messages, clients.ChatMessage{Role: "assistant", Content: m.Content})
		}
	}
	return messages
}

// buildWorkflowSystemPrompt 构造工作流生成所需的 system prompt。
func (a *App) buildWorkflowSystemPrompt(session core.AISession) string {
	nodeTypes := summarizeNodeTypes(a.GetNodeTypes())
	nodeJSON, _ := json.Marshal(nodeTypes)

	var envSums []envSummary
	if a.environmentStore != nil {
		if envs, err := a.environmentStore.List(); err == nil {
			envSums = summarizeEnvironments(envs)
		}
	}
	envJSON, _ := json.Marshal(envSums)

	preference := map[string]string{
		"environment_id": session.EnvironmentID,
		"ssh_config_id":  session.ConfigID,
	}
	prefJSON, _ := json.Marshal(preference)

	var sb strings.Builder
	sb.WriteString("你是 OpsEngine 工作流生成助手，根据用户的运维需求生成可直接执行的工作流。\n\n")
	sb.WriteString("# 可用节点类型（按目录组合，不能编造）\n")
	sb.Write(nodeJSON)
	sb.WriteString("\n\n# 已配置环境（环境/配置的 id 必须严格引用，不可编造）\n")
	sb.Write(envJSON)
	sb.WriteString("\n\n# 用户偏好（如生成包含 env_connect_ssh 节点，优先填入这里的 id）\n")
	sb.Write(prefJSON)
	sb.WriteString("\n\n# 输出规范\n")
	sb.WriteString(`只允许返回单个 JSON，禁止 Markdown、禁止解释文字。结构如下：
{
  "name": "工作流名称",
  "description": "一句话描述",
  "variables": [],
  "nodes": [
    {"id":"n1","type_id":"system_ready","config":{},"position":{"x":80,"y":120}}
  ],
  "edges": [
    {"from":{"node":"n1","port":"exec_out"},"to":{"node":"n2","port":"exec_in"}}
  ],
  "notes": []
}

# 硬约束
- 必须有且仅有一个 system_ready 节点作为入口
- 只能使用上面列出的 type_id，未列出的禁止使用
- 节点 id 用 n1/n2 等临时占位，后端会替换成 UUID
- environment_id / config_id 必须在已配置环境中存在
- exec_out 输出端口最多连一条边；数据输入端口最多连一条入边
- 端口 id 必须使用节点类型定义中的端口 id，不能编造
- 不确定的字段留空，并把疑问写入 notes 数组让用户补全
- 不要在工作流中加入会破坏系统的命令（rm -rf / mkfs / shutdown 等）
`)
	return sb.String()
}

// parseGeneratedWorkflow 从模型回复中提取并解析 JSON 草案。
func parseGeneratedWorkflow(reply string) (aiGeneratedWorkflow, error) {
	text := strings.TrimSpace(reply)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return aiGeneratedWorkflow{}, errors.New("AI 返回内容不是 JSON")
	}
	var draft aiGeneratedWorkflow
	if err := json.Unmarshal([]byte(text[start:end+1]), &draft); err != nil {
		return aiGeneratedWorkflow{}, fmt.Errorf("解析 AI 工作流草案失败: %w", err)
	}
	if len(draft.Nodes) == 0 {
		return aiGeneratedWorkflow{}, errors.New("AI 返回的节点列表为空")
	}
	return draft, nil
}

// materializeWorkflow 把模型草案转成 core.WorkflowDef：临时 id 重写成 UUID、
// 节点类型查表、边重新映射，最后跑一次工作流校验。
func (a *App) materializeWorkflow(draft aiGeneratedWorkflow) (core.WorkflowDef, error) {
	idMap := make(map[string]string, len(draft.Nodes))
	nodes := make([]core.NodeInstance, 0, len(draft.Nodes))
	for _, n := range draft.Nodes {
		tempID := strings.TrimSpace(n.ID)
		if tempID == "" {
			return core.WorkflowDef{}, errors.New("AI 节点缺少 id")
		}
		if _, dup := idMap[tempID]; dup {
			return core.WorkflowDef{}, fmt.Errorf("AI 节点 id 重复: %s", tempID)
		}
		if err := a.checkNodeTypeExists(n.TypeID); err != nil {
			return core.WorkflowDef{}, err
		}
		instanceID := uuid.New().String()
		idMap[tempID] = instanceID
		cfg := n.Config
		if cfg == nil {
			cfg = map[string]any{}
		}
		nodes = append(nodes, core.NodeInstance{
			InstanceID: instanceID,
			TypeID:     n.TypeID,
			Config:     cfg,
			Position:   core.Position{X: n.Position.X, Y: n.Position.Y},
		})
	}

	edges := make([]core.EdgeConfig, 0, len(draft.Edges))
	for _, e := range draft.Edges {
		fromID, ok := idMap[e.From.Node]
		if !ok {
			return core.WorkflowDef{}, fmt.Errorf("边引用未知节点: %s", e.From.Node)
		}
		toID, ok := idMap[e.To.Node]
		if !ok {
			return core.WorkflowDef{}, fmt.Errorf("边引用未知节点: %s", e.To.Node)
		}
		edges = append(edges, core.EdgeConfig{
			From: core.PortRef{Node: fromID, Port: strings.TrimSpace(e.From.Port)},
			To:   core.PortRef{Node: toID, Port: strings.TrimSpace(e.To.Port)},
		})
	}

	name := strings.TrimSpace(draft.Name)
	if name == "" {
		name = "AI 生成工作流"
	}
	variables := draft.Variables
	if variables == nil {
		variables = []core.VariableDef{}
	}
	workflow := core.WorkflowDef{
		ID:          uuid.New().String(),
		Name:        name,
		Description: strings.TrimSpace(draft.Description),
		Variables:   variables,
		Nodes:       nodes,
		Edges:       edges,
	}
	if err := engine.ValidateWorkflow(workflow); err != nil {
		return core.WorkflowDef{}, fmt.Errorf("AI 生成工作流校验失败: %w", err)
	}
	return workflow, nil
}

// checkNodeTypeExists 验证 type_id 是注册节点或现存集合。
func (a *App) checkNodeTypeExists(typeID string) error {
	typeID = strings.TrimSpace(typeID)
	if typeID == "" {
		return errors.New("节点缺少 type_id")
	}
	if strings.HasPrefix(typeID, assembleTypePrefix) {
		if a.assembleStore == nil {
			return fmt.Errorf("集合存储未初始化，无法引用 %s", typeID)
		}
		asmID := strings.TrimPrefix(typeID, assembleTypePrefix)
		if _, err := a.assembleStore.Get(asmID); err != nil {
			return fmt.Errorf("引用了不存在的集合: %s", asmID)
		}
		return nil
	}
	if _, ok := engine.Lookup(typeID); !ok {
		return fmt.Errorf("未知节点类型: %s", typeID)
	}
	return nil
}

// ── prompt 摘要结构 ───────────────────────────────────────────

// nodeTypeSummary 是注入 prompt 的精简节点描述。
type nodeTypeSummary struct {
	TypeID       string          `json:"type_id"`
	Name         string          `json:"name"`
	Kind         core.NodeKind   `json:"kind"`
	Description  string          `json:"description,omitempty"`
	InputPorts   []portSummary   `json:"in,omitempty"`
	OutputPorts  []portSummary   `json:"out,omitempty"`
	ConfigSchema []schemaSummary `json:"config,omitempty"`
}

// portSummary 是端口的精简描述。
type portSummary struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// schemaSummary 是配置字段的精简描述。
type schemaSummary struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
	Default  any    `json:"default,omitempty"`
}

// envSummary 是注入 prompt 的环境摘要（不带任何敏感字段）。
type envSummary struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	Configs []envConfigSummary `json:"configs,omitempty"`
}

// envConfigSummary 是单条环境配置的精简描述。
type envConfigSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// summarizeNodeTypes 把完整节点定义压缩成 prompt 友好格式。
func summarizeNodeTypes(defs []core.NodeTypeDef) []nodeTypeSummary {
	out := make([]nodeTypeSummary, 0, len(defs))
	for _, d := range defs {
		out = append(out, nodeTypeSummary{
			TypeID:       d.TypeID,
			Name:         d.DisplayName,
			Kind:         d.NodeKind,
			Description:  d.Description,
			InputPorts:   summarizePorts(d.InputPorts),
			OutputPorts:  summarizePorts(d.OutputPorts),
			ConfigSchema: summarizeSchema(d.ConfigSchema),
		})
	}
	return out
}

// summarizePorts 抽取端口的 id 与类型。
func summarizePorts(ports []core.PortDef) []portSummary {
	if len(ports) == 0 {
		return nil
	}
	out := make([]portSummary, 0, len(ports))
	for _, p := range ports {
		out = append(out, portSummary{ID: p.ID, Type: string(p.PortType)})
	}
	return out
}

// summarizeSchema 抽取配置字段的 id/type/required/default。
func summarizeSchema(fields []core.FieldSchema) []schemaSummary {
	if len(fields) == 0 {
		return nil
	}
	out := make([]schemaSummary, 0, len(fields))
	for _, f := range fields {
		out = append(out, schemaSummary{
			ID:       f.ID,
			Type:     f.Type,
			Required: f.Required,
			Default:  f.Default,
		})
	}
	return out
}

// summarizeEnvironments 把环境列表脱敏后注入 prompt（只保留 id/name/kind）。
func summarizeEnvironments(envs []core.EnvironmentDef) []envSummary {
	out := make([]envSummary, 0, len(envs))
	for _, env := range envs {
		configs := make([]envConfigSummary, 0, len(env.Configs))
		for _, c := range env.Configs {
			configs = append(configs, envConfigSummary{ID: c.ID, Name: c.Name, Kind: string(c.Kind)})
		}
		out = append(out, envSummary{ID: env.ID, Name: env.Name, Configs: configs})
	}
	return out
}

// ── 工具函数 ───────────────────────────────────────────────

// isFirstUserMessage 判断 session.Messages 中是否仅有一条 user 消息（即刚追加的那条）。
func isFirstUserMessage(session core.AISession) bool {
	count := 0
	for _, m := range session.Messages {
		if m.Role == core.AIMessageRoleUser {
			count++
			if count > 1 {
				return false
			}
		}
	}
	return count == 1
}

// makeSessionTitle 从用户第一条消息截取标题，限制 30 个字符。
func makeSessionTitle(message string) string {
	message = strings.TrimSpace(message)
	runes := []rune(message)
	if len(runes) > 30 {
		return string(runes[:30]) + "…"
	}
	if len(runes) == 0 {
		return "新会话"
	}
	return message
}

// loadAISettings 从本地 TOML 文件读取配置。
func loadAISettings() (AISettings, error) {
	settings := defaultAISettings()
	content, err := os.ReadFile(AISettingsFile)
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return AISettings{}, fmt.Errorf("读取 AI 配置失败: %w", err)
	}
	if _, err := toml.Decode(string(content), &settings); err != nil {
		return AISettings{}, fmt.Errorf("解析 AI 配置失败: %w", err)
	}
	return normalizeAISettings(settings), nil
}

// defaultAISettings 返回默认连接配置（指向 DeepSeek）。
func defaultAISettings() AISettings {
	return AISettings{
		DeepSeekBaseURL: clients.DefaultDeepSeekBaseURL,
		DeepSeekModel:   clients.DefaultDeepSeekModel,
		TimeoutSeconds:  60,
	}
}

// normalizeAISettings 补齐默认值并清理首尾空白。
func normalizeAISettings(settings AISettings) AISettings {
	settings.DeepSeekAPIKey = strings.TrimSpace(settings.DeepSeekAPIKey)
	settings.DeepSeekBaseURL = strings.TrimSpace(settings.DeepSeekBaseURL)
	settings.DeepSeekModel = strings.TrimSpace(settings.DeepSeekModel)
	if settings.DeepSeekBaseURL == "" {
		settings.DeepSeekBaseURL = clients.DefaultDeepSeekBaseURL
	}
	if settings.DeepSeekModel == "" {
		settings.DeepSeekModel = clients.DefaultDeepSeekModel
	}
	if settings.TimeoutSeconds <= 0 {
		settings.TimeoutSeconds = 60
	}
	return settings
}
