// AI 助手 Wails 适配层。
// 本文件只做三件事：
//  1. AI 设置和会话的本地存储 CRUD
//  2. Wails RPC 入口（GetXxx / UpdateXxx / CreateXxx / StartAIAssistant）
//  3. 把外部依赖（LLM/store/Wails 事件）适配成 internal/agent/runtime 的接口后委托执行
//
// 所有 AI 业务算法（意图判别、prompt 构造、上下文管理、工作流落地、巡检）都在 internal/agent/* 子包。
// 本文件中不应再出现 prompt 拼接、JSON 解析、节点摘要等业务逻辑。

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"OpsEngine/internal/agent/runtime"
	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	"github.com/BurntSushi/toml"
	"github.com/google/uuid"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// AISettingsFile 是本地 AI 配置文件路径。
const AISettingsFile = "data/settings/ai.toml"

// AISettings 保存外部大模型连接参数。字段名沿用 deepseek_* 以兼容已有配置文件，
// 但底层走的是 OpenAI 兼容协议，base_url 换成任何兼容端点都能用。
type AISettings struct {
	DeepSeekAPIKey  string `json:"deepseek_api_key"  toml:"deepseek_api_key"`
	DeepSeekBaseURL string `json:"deepseek_base_url" toml:"deepseek_base_url"`
	DeepSeekModel   string `json:"deepseek_model"    toml:"deepseek_model"`
	TimeoutSeconds  int    `json:"timeout_seconds"   toml:"timeout_seconds"`
	// ApplyMode 控制 AI 修改已有资产的落盘策略：confirm（默认，先确认）/ auto（直接保存）。
	ApplyMode string `json:"apply_mode" toml:"apply_mode"`
}

// AIAssistantRequest 是统一 AI 助手请求（Wails 入口签名）。
type AIAssistantRequest struct {
	RequestID      string `json:"request_id"`
	SessionID      string `json:"session_id"`
	Operation      string `json:"operation"`
	Message        string `json:"message"`
	TargetConfigID string `json:"target_config_id,omitempty"`
	ArtifactType   string `json:"artifact_type,omitempty"`
	ArtifactID     string `json:"artifact_id,omitempty"`
	// ExecutionID 指向要修复的失败执行，operation=fix_execution 时必填。
	ExecutionID string `json:"execution_id,omitempty"`
	// ModeHint 是 `/` 模式选择器注入的本轮行为指令（画图/计划/苏格拉底等）。
	ModeHint string `json:"mode_hint,omitempty"`
}

// AIAssistantEvent 是后端推送给前端的 AI 助手事件。
// 字段与 runtime.Event 对齐，wailsEmitter 把后者转译为前者。
type AIAssistantEvent struct {
	RequestID     string                 `json:"request_id"`
	SessionID     string                 `json:"session_id,omitempty"`
	Type          string                 `json:"type"`
	Text          string                 `json:"text,omitempty"`
	View          *core.AIViewPayload    `json:"view,omitempty"`
	WorkflowID    string                 `json:"workflow_id,omitempty"`
	WorkflowName  string                 `json:"workflow_name,omitempty"`
	AssembleID    string                 `json:"assemble_id,omitempty"`
	AssembleName  string                 `json:"assemble_name,omitempty"`
	ArtifactType  string                 `json:"artifact_type,omitempty"`
	ActionType    string                 `json:"action_type,omitempty"`
	DocID         string                 `json:"doc_id,omitempty"`
	DocTitle      string                 `json:"doc_title,omitempty"`
	NodeCount     int                    `json:"node_count,omitempty"`
	ChangeSummary string                 `json:"change_summary,omitempty"`
	TargetOptions []runtime.TargetOption `json:"target_options,omitempty"`
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
	llm := a.newLLMAdapter(settings)
	return llm.Chat([]clients.ChatMessage{
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

// CreateAISession 创建空会话。
//   - environmentID 为空：通用会话，可生成/修改集合和工作流等可复用资产
//   - configID 可选：空字符串 → 环境级会话；非空 → 校验是该环境下的 SSH 配置并落到 config 范围
//
// 环境级会话让 Agent 看到整个环境，适合架构分析、多机巡检等；
// config 级会话强绑某个 SSH 配置，等于旧行为。
func (a *App) CreateAISession(environmentID, configID, title string) (core.AISession, error) {
	environmentID = strings.TrimSpace(environmentID)
	configID = strings.TrimSpace(configID)
	scope := core.AISessionScopeGeneral
	if environmentID != "" {
		if err := a.validateEnvironment(environmentID); err != nil {
			return core.AISession{}, err
		}
		scope = core.AISessionScopeEnvironment
	}
	if configID != "" {
		if environmentID == "" {
			return core.AISession{}, errors.New("选择 SSH 配置前请先选择环境")
		}
		if err := a.validateSSHConfig(environmentID, configID); err != nil {
			return core.AISession{}, err
		}
		scope = core.AISessionScopeConfig
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
		Scope:         scope,
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

// UpdateAISessionContext 更新已有会话的环境和 SSH 目标。
// 通用会话遇到巡检、排障等真实环境任务时，可先补选上下文再重试。
func (a *App) UpdateAISessionContext(id, environmentID, configID string) error {
	if a.aiSessionStore == nil {
		return errors.New("会话存储未初始化")
	}
	id = strings.TrimSpace(id)
	environmentID = strings.TrimSpace(environmentID)
	configID = strings.TrimSpace(configID)
	if id == "" {
		return errors.New("session_id 不能为空")
	}
	scope := core.AISessionScopeGeneral
	if environmentID != "" {
		if err := a.validateEnvironment(environmentID); err != nil {
			return err
		}
		scope = core.AISessionScopeEnvironment
	}
	if configID != "" {
		if environmentID == "" {
			return errors.New("选择 SSH 配置前请先选择环境")
		}
		if err := a.validateSSHConfig(environmentID, configID); err != nil {
			return err
		}
		scope = core.AISessionScopeConfig
	}
	session, err := a.aiSessionStore.Get(id)
	if err != nil {
		return err
	}
	changed := session.EnvironmentID != environmentID || session.ConfigID != configID || session.Scope != scope
	session.EnvironmentID = environmentID
	session.ConfigID = configID
	session.Scope = scope
	if changed {
		session.ContextPrefetched = false
		session.Messages = dropHiddenSystemMessages(session.Messages)
	}
	session.UpdatedAt = time.Now()
	return a.aiSessionStore.Save(session)
}

// dropHiddenSystemMessages 移除旧环境预取快照，避免切换上下文后继续把旧机器信息注入 LLM。
func dropHiddenSystemMessages(messages []core.AISessionMessage) []core.AISessionMessage {
	out := messages[:0]
	for _, message := range messages {
		if message.Role == core.AIMessageRoleSystem && message.Hidden {
			continue
		}
		out = append(out, message)
	}
	return out
}

// DeleteAISession 删除会话。
func (a *App) DeleteAISession(id string) error {
	if a.aiSessionStore == nil {
		return errors.New("会话存储未初始化")
	}
	return a.aiSessionStore.Delete(id)
}

// SetAISessionActiveArtifact 进入 artifact 编辑模式（持久化到会话，刷新不丢）。
func (a *App) SetAISessionActiveArtifact(sessionID, artifactType, artifactID, artifactName string) error {
	if a.aiSessionStore == nil {
		return errors.New("会话存储未初始化")
	}
	sessionID = strings.TrimSpace(sessionID)
	artifactType = strings.TrimSpace(artifactType)
	artifactID = strings.TrimSpace(artifactID)
	artifactName = strings.TrimSpace(artifactName)
	if sessionID == "" || artifactType == "" || artifactID == "" {
		return errors.New("session_id、artifact_type、artifact_id 不能为空")
	}
	if artifactType != "workflow" && artifactType != "assemble" {
		return errors.New("artifact_type 必须是 workflow 或 assemble")
	}
	session, err := a.aiSessionStore.Get(sessionID)
	if err != nil {
		return err
	}
	session.ActiveArtifactType = artifactType
	session.ActiveArtifactID = artifactID
	session.ActiveArtifactName = artifactName
	session.UpdatedAt = time.Now()
	return a.aiSessionStore.Save(session)
}

// ClearAISessionActiveArtifact 退出 artifact 编辑模式。
func (a *App) ClearAISessionActiveArtifact(sessionID string) error {
	if a.aiSessionStore == nil {
		return errors.New("会话存储未初始化")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session_id 不能为空")
	}
	session, err := a.aiSessionStore.Get(sessionID)
	if err != nil {
		return err
	}
	session.ActiveArtifactType = ""
	session.ActiveArtifactID = ""
	session.ActiveArtifactName = ""
	session.UpdatedAt = time.Now()
	return a.aiSessionStore.Save(session)
}

// ── 统一助手入口 ────────────────────────────────────────────

// StartAIAssistant 把请求委托给 Runtime。本方法只负责：
//  1. 校验会话存在且 SSH 配置合法
//  2. 加载 AI 设置并构造 LLM 适配器
//  3. 组装 Runtime 依赖
//  4. 调用 Runtime.Run
//
// 业务流程（意图判别、上下文、prompt、handler）全部下沉到 internal/agent/runtime。
func (a *App) StartAIAssistant(req AIAssistantRequest) error {
	requestID := strings.TrimSpace(req.RequestID)
	sessionID := strings.TrimSpace(req.SessionID)
	if requestID == "" {
		return errors.New("request_id 不能为空")
	}
	if a.aiSessionStore == nil {
		a.emitAIAssistantText(requestID, sessionID, "error", "会话存储未初始化")
		return nil
	}
	session, err := a.aiSessionStore.Get(sessionID)
	if err != nil {
		a.emitAIAssistantText(requestID, sessionID, "error", err.Error())
		return nil
	}
	// 通用会话不校验环境；环境/config 会话仍保持原有校验。
	if strings.TrimSpace(session.EnvironmentID) != "" {
		if err := a.validateEnvironment(session.EnvironmentID); err != nil {
			a.emitAIAssistantText(requestID, sessionID, "error", err.Error())
			return nil
		}
	}
	if session.ConfigID != "" {
		if err := a.validateSSHConfig(session.EnvironmentID, session.ConfigID); err != nil {
			a.emitAIAssistantText(requestID, sessionID, "error", err.Error())
			return nil
		}
	}
	targetConfigID := strings.TrimSpace(req.TargetConfigID)
	if targetConfigID != "" {
		if strings.TrimSpace(session.EnvironmentID) == "" {
			a.emitAIAssistantText(requestID, sessionID, "error", "通用会话需要先选择环境后才能选择 SSH 目标")
			return nil
		}
		if err := a.validateSSHConfig(session.EnvironmentID, targetConfigID); err != nil {
			a.emitAIAssistantText(requestID, sessionID, "error", err.Error())
			return nil
		}
	}
	settings, err := loadAISettings()
	if err != nil {
		a.emitAIAssistantText(requestID, sessionID, "error", err.Error())
		return nil
	}

	rt := &runtime.Runtime{
		Sessions:     a.aiSessionStore,
		Workflows:    a.workflowStore,
		Assembles:    a.assembleStore,
		OpsDocs:      a.opsDocStore,
		Environments: a.lookupEnvironment,
		EnvList:      a.listEnvironmentsForPrompt,
		Nodes:        a.GetNodeTypes,
		NodeChecker:  a.checkNodeTypeExists,
		Executions:   a.getExecutionRecord,
		LLM:          a.newLLMAdapter(settings),
		Emit:         runtime.EmitterFunc(a.emitRuntimeEvent),
		Tools:        a.toolRegistry,
		ApplyMode:    settings.ApplyMode,
	}
	return rt.Run(runtime.Request{
		RequestID:      requestID,
		SessionID:      sessionID,
		Operation:      req.Operation,
		Message:        req.Message,
		TargetConfigID: targetConfigID,
		ArtifactType:   req.ArtifactType,
		ArtifactID:     req.ArtifactID,
		ExecutionID:    req.ExecutionID,
		ModeHint:       req.ModeHint,
	})
}

// ApplyAIPendingDraft 应用会话中的待确认 AI 修改草案，返回保存后的工作流。
// 基线漂移（草案生成后被手工修改）时返回错误并保留草案。
func (a *App) ApplyAIPendingDraft(sessionID string) (core.WorkflowDef, error) {
	rt := &runtime.Runtime{Sessions: a.aiSessionStore, Workflows: a.workflowStore}
	return rt.ApplyPendingDraft(strings.TrimSpace(sessionID))
}

// DiscardAIPendingDraft 放弃会话中的待确认 AI 修改草案（幂等）。
func (a *App) DiscardAIPendingDraft(sessionID string) error {
	rt := &runtime.Runtime{Sessions: a.aiSessionStore, Workflows: a.workflowStore}
	return rt.DiscardPendingDraft(strings.TrimSpace(sessionID))
}

// ── Runtime 依赖适配 ───────────────────────────────────────

// getExecutionRecord 适配 runtime.ExecutionGetter 与工具 ExecutionGet：内存优先取执行记录。
func (a *App) getExecutionRecord(executionID string) (core.ExecutionRecord, error) {
	if a.engine == nil {
		return core.ExecutionRecord{}, errors.New("执行引擎未初始化")
	}
	rec, ok := a.engine.GetRecord(executionID)
	if !ok {
		return core.ExecutionRecord{}, fmt.Errorf("执行 %s 不存在", executionID)
	}
	return rec, nil
}

// lookupEnvironment 适配 runtime.EnvironmentLookup。
func (a *App) lookupEnvironment(environmentID string) (core.EnvironmentDef, error) {
	if a.environmentStore == nil {
		return core.EnvironmentDef{}, errors.New("环境存储未初始化")
	}
	return a.environmentStore.Get(environmentID)
}

// listEnvironmentsForPrompt 适配 runtime.EnvironmentLister。store 缺失时返回空切片不致命。
func (a *App) listEnvironmentsForPrompt() ([]core.EnvironmentDef, error) {
	if a.environmentStore == nil {
		return nil, nil
	}
	return a.environmentStore.List()
}

// checkNodeTypeExists 适配 runtime.NodeTypeChecker：判 type_id 是注册节点或现存集合。
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

// emitRuntimeEvent 把 runtime.Event 转译为前端期望的 AIAssistantEvent，再走 Wails 事件总线。
func (a *App) emitRuntimeEvent(e runtime.Event) {
	if a.ctx == nil {
		return
	}
	wailsruntime.EventsEmit(a.ctx, "ai:assistant", AIAssistantEvent{
		RequestID:     e.RequestID,
		SessionID:     e.SessionID,
		Type:          string(e.Type),
		Text:          e.Text,
		View:          e.View,
		WorkflowID:    e.WorkflowID,
		WorkflowName:  e.WorkflowName,
		AssembleID:    e.AssembleID,
		AssembleName:  e.AssembleName,
		ArtifactType:  e.ArtifactType,
		ActionType:    e.ActionType,
		DocID:         e.DocID,
		DocTitle:      e.DocTitle,
		NodeCount:     e.NodeCount,
		ChangeSummary: e.ChangeSummary,
		TargetOptions: e.TargetOptions,
	})
}

// emitAIAssistantText 是参数级错误推送的便捷函数，用于 StartAIAssistant 入口校验阶段
// （那时还没创建 Runtime，无法走 Runtime.Emit）。
func (a *App) emitAIAssistantText(requestID, sessionID, eventType, text string) {
	a.emitRuntimeEvent(runtime.Event{
		RequestID: requestID, SessionID: sessionID,
		Type: runtime.EventType(eventType), Text: text,
	})
}

// validateEnvironment 校验环境存在。通用 AI 会话可不绑定环境，调用前需判断是否为空。
func (a *App) validateEnvironment(environmentID string) error {
	environmentID = strings.TrimSpace(environmentID)
	if environmentID == "" {
		return errors.New("请先选择目标环境")
	}
	if a.environmentStore == nil {
		return errors.New("环境存储未初始化")
	}
	if _, err := a.environmentStore.Get(environmentID); err != nil {
		return err
	}
	return nil
}

// validateSSHConfig 校验指定 ConfigID 在环境内存在且 kind=ssh。
// 仅在 config 级会话或显式追加目标时调用。
func (a *App) validateSSHConfig(environmentID, configID string) error {
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

// ── LLM 适配 ───────────────────────────────────────────────

// llmAdapter 把 clients.LLMClient 包装成 runtime.LLMProvider，
// 负责按 settings.TimeoutSeconds 构造带超时的 context，并继承 App.ctx 用于取消传播。
type llmAdapter struct {
	parent   context.Context
	client   clients.LLMClient
	timeout  time.Duration
	settings AISettings
}

// newLLMAdapter 根据当前 settings 构造一个 LLMProvider 实现。
func (a *App) newLLMAdapter(settings AISettings) *llmAdapter {
	settings = normalizeAISettings(settings)
	timeout := time.Duration(settings.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	return &llmAdapter{
		parent:  parent,
		timeout: timeout,
		client: clients.LLMClient{
			BaseURL:        settings.DeepSeekBaseURL,
			APIKey:         settings.DeepSeekAPIKey,
			Model:          settings.DeepSeekModel,
			TimeoutSeconds: settings.TimeoutSeconds,
		},
		settings: settings,
	}
}

// Chat 实现 runtime.LLMProvider。
func (l *llmAdapter) Chat(messages []clients.ChatMessage) (string, error) {
	ctx, cancel := context.WithTimeout(l.parent, l.timeout)
	defer cancel()
	return l.client.Chat(ctx, messages)
}

// ChatStream 实现 runtime.LLMProvider。
func (l *llmAdapter) ChatStream(messages []clients.ChatMessage, onDelta func(string)) (string, error) {
	ctx, cancel := context.WithTimeout(l.parent, l.timeout)
	defer cancel()
	return l.client.ChatStream(ctx, messages, onDelta)
}

// ChatWithTools 实现 runtime.LLMProvider；走 OpenAI function calling 协议。
func (l *llmAdapter) ChatWithTools(messages []clients.ChatMessage, toolSpecs []clients.ToolSpec) (clients.ChatCompletion, error) {
	ctx, cancel := context.WithTimeout(l.parent, l.timeout)
	defer cancel()
	return l.client.ChatWithTools(ctx, messages, toolSpecs)
}

// ChatWithToolsStream 实现 runtime.LLMProvider；流式工具调用让用户看到实时 content。
func (l *llmAdapter) ChatWithToolsStream(messages []clients.ChatMessage, toolSpecs []clients.ToolSpec, onContent func(string)) (clients.ChatCompletion, error) {
	ctx, cancel := context.WithTimeout(l.parent, l.timeout)
	defer cancel()
	return l.client.ChatWithToolsStream(ctx, messages, toolSpecs, onContent)
}

// ── 设置加载与默认值 ───────────────────────────────────────

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
// TimeoutSeconds 默认 120：生成集合 / 复杂工作流时 60s 经常不够（DeepSeek 大量推理时尾部读取阶段也会触发 deadline）。
func defaultAISettings() AISettings {
	return AISettings{
		DeepSeekBaseURL: clients.DefaultDeepSeekBaseURL,
		DeepSeekModel:   clients.DefaultDeepSeekModel,
		TimeoutSeconds:  120,
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
		settings.TimeoutSeconds = 120
	}
	// 默认 confirm：AI 覆盖已有资产前需要用户确认（见 P3 计划）。
	if settings.ApplyMode != "auto" {
		settings.ApplyMode = runtime.ApplyModeConfirm
	}
	return settings
}
