// OpsDoc Wails 适配层：文档 CRUD + 巡检报告生成入口。
// 文档存储和报告事实抽取分别在 internal/store 和 internal/agent/report 中；本文件只做参数校验
// 与依赖装配，把 store / LLM / 环境查询桥接到 report 包。

package main

import (
	"errors"
	"fmt"

	"OpsEngine/internal/agent/report"
	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
)

// ListOpsDocs 返回所有文档摘要，按 UpdatedAt 倒序。
func (a *App) ListOpsDocs() ([]core.OpsDocSummary, error) {
	if a.opsDocStore == nil {
		return []core.OpsDocSummary{}, nil
	}
	return a.opsDocStore.List()
}

// GetOpsDoc 按 ID 加载完整文档（含 Markdown 正文）。
func (a *App) GetOpsDoc(id string) (core.OpsDoc, error) {
	if a.opsDocStore == nil {
		return core.OpsDoc{}, errors.New("文档存储未初始化")
	}
	return a.opsDocStore.Get(id)
}

// DeleteOpsDoc 删除文档（元数据 + 正文）。
func (a *App) DeleteOpsDoc(id string) error {
	if a.opsDocStore == nil {
		return errors.New("文档存储未初始化")
	}
	return a.opsDocStore.Delete(id)
}

// SaveAssistantMessageAsDoc 把一条 assistant 消息（含工具调用 progress + 正文）落成 OpsDoc。
// Kind 由消息 Intent 推断（troubleshoot → troubleshooting，其他 → architecture/troubleshooting fallback）。
//   - chat 消息默认归类为 troubleshooting（用户主动保存为报告通常是有分析结论的对话）
//   - generate_workflow / inspect_server 消息一般已经有工作流入口，不建议保存为文档，但调用时仍允许
func (a *App) SaveAssistantMessageAsDoc(sessionID, messageID string) (core.OpsDoc, error) {
	if a.aiSessionStore == nil || a.opsDocStore == nil {
		return core.OpsDoc{}, errors.New("依赖存储未初始化")
	}
	session, err := a.aiSessionStore.Get(sessionID)
	if err != nil {
		return core.OpsDoc{}, err
	}
	var msg core.AISessionMessage
	found := false
	for _, m := range session.Messages {
		if m.ID == messageID {
			msg = m
			found = true
			break
		}
	}
	if !found {
		return core.OpsDoc{}, fmt.Errorf("会话 %s 中未找到消息 %s", sessionID, messageID)
	}
	env := core.EnvironmentDef{ID: session.EnvironmentID}
	if a.environmentStore != nil {
		if loaded, err := a.environmentStore.Get(session.EnvironmentID); err == nil {
			env = loaded
		}
	}
	kind := opsDocKindForIntent(msg.Intent)
	doc, err := report.FromSessionMessage(env, session, msg, kind)
	if err != nil {
		return core.OpsDoc{}, err
	}
	if err := a.opsDocStore.Save(doc); err != nil {
		return core.OpsDoc{}, fmt.Errorf("保存报告失败: %w", err)
	}
	return doc, nil
}

// opsDocKindForIntent 把消息 Intent 字符串映射成 OpsDocKind。
// 默认归到 troubleshooting，因为这是用户主动"保存为报告"最常见的来源；
// 未来加 architecture intent 时在这里扩展。
func opsDocKindForIntent(intent string) core.OpsDocKind {
	switch intent {
	case "troubleshoot":
		return core.OpsDocKindTroubleshooting
	case "inspect_server":
		return core.OpsDocKindInspection
	default:
		return core.OpsDocKindTroubleshooting
	}
}

// GenerateInspectionReport 根据执行记录生成巡检报告 OpsDoc 并落盘。
//   - executionID 必填
//   - 若 AI 设置中有可用 API Key 则做 LLM 风险分析；否则退化为纯事实报告
//   - 报告失败不致命：事实层产物一定可用，LLM 失败原因写进风险段
func (a *App) GenerateInspectionReport(executionID string) (core.OpsDoc, error) {
	if a.executionStore == nil || a.workflowStore == nil || a.environmentStore == nil || a.opsDocStore == nil {
		return core.OpsDoc{}, errors.New("依赖存储未初始化")
	}
	rec, err := a.executionStore.Get(executionID)
	if err != nil {
		return core.OpsDoc{}, fmt.Errorf("加载执行记录失败: %w", err)
	}
	wf := rec.Snapshot.Workflow
	if wf.ID == "" {
		// 极端兜底：执行记录里没存工作流快照，回退到 store 现取
		wf, err = a.workflowStore.Get(rec.WorkflowID)
		if err != nil {
			return core.OpsDoc{}, fmt.Errorf("加载工作流定义失败: %w", err)
		}
	}
	env := core.EnvironmentDef{} // 环境信息可选；用于报告摘要展示，缺失不致命
	if envID := inferEnvironmentID(wf); envID != "" {
		if loaded, err := a.environmentStore.Get(envID); err == nil {
			env = loaded
		}
	}

	doc, err := report.GenerateInspection(env, wf, rec, core.AISession{}, a.makeReportSummarizer())
	if err != nil {
		return core.OpsDoc{}, err
	}
	if err := a.opsDocStore.Save(doc); err != nil {
		return core.OpsDoc{}, fmt.Errorf("保存报告失败: %w", err)
	}
	return doc, nil
}

// makeReportSummarizer 构造 LLM 风险分析回调；API Key 未配置时返回 nil 走纯事实路径。
func (a *App) makeReportSummarizer() report.Summarizer {
	settings, err := loadAISettings()
	if err != nil || settings.DeepSeekAPIKey == "" {
		return nil
	}
	llm := a.newLLMAdapter(settings)
	return func(messages []clients.ChatMessage) (string, error) {
		return llm.Chat(messages)
	}
}

// inferEnvironmentID 从工作流中查找第一个含 environment_id 的节点配置，作为报告关联的环境。
// 巡检工作流必有 env_connect_ssh 节点，所以这一路兜底足够；
// 其他形态找不到时返回空串，报告里仅显示工作流名称。
func inferEnvironmentID(wf core.WorkflowDef) string {
	for _, n := range wf.Nodes {
		if v, ok := n.Config["environment_id"].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
