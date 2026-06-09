// 环境架构分析（intent.KindAnalyzeArchitecture）。
//
// 流水线（文档 §3.1）：
//   inventory → collect (SSH 拨号采集) → topology JSON → LLM 说明 → Mermaid → OpsDoc
//
// 与 inspection 不同的是：架构分析采集本身就是只读 + 并发可控，
// 不需要"生成工作流让用户确认"——直接产文档资产即可。

package runtime

import (
	"fmt"
	"time"

	"OpsEngine/internal/agent/architecture"
	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"

	"github.com/google/uuid"
)

// handleArchitecture 处理 intent.KindAnalyzeArchitecture。
func (r *Runtime) handleArchitecture(req Request, session core.AISession) {
	progress := []string{}

	if r.OpsDocs == nil {
		r.emitError(req.RequestID, session.ID, "文档存储未注入，无法生成架构报告")
		return
	}

	r.emitProgress(req.RequestID, session.ID, "正在加载环境资产清单", &progress)
	env, err := r.Environments(session.EnvironmentID)
	if err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	r.emitProgress(req.RequestID, session.ID, "正在采集环境拓扑（SSH 只读）", &progress)
	graph, err := architecture.CollectFromEnvironment(
		architecture.EnvironmentLookup(r.Environments),
		session.EnvironmentID,
		func(text string) {
			r.emitProgress(req.RequestID, session.ID, "🔧 "+text, &progress)
		},
	)
	if err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	if r.LLM != nil {
		r.emitProgress(req.RequestID, session.ID, "正在生成架构说明", &progress)
	} else {
		r.emitProgress(req.RequestID, session.ID, "LLM 未配置，跳过架构说明段", &progress)
	}
	doc, err := architecture.GenerateArchitectureDoc(env, graph, r.makeArchitectureSummarizer())
	if err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	r.emitProgress(req.RequestID, session.ID, "正在保存架构报告", &progress)
	if err := r.OpsDocs.Save(doc); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}

	// 推送 doc 事件，前端据此显示"查看文档"按钮
	r.Emit.Emit(Event{
		RequestID: req.RequestID, SessionID: session.ID,
		Type:     EventDoc,
		DocID:    doc.ID,
		DocTitle: doc.Title,
	})

	session.Messages = append(session.Messages, core.AISessionMessage{
		ID:   uuid.New().String(),
		Role: core.AIMessageRoleAssistant,
		Content: fmt.Sprintf(
			"已完成环境「%s」的架构分析，采集到 %d 台主机、%d 条监听端口、%d 个容器。报告已保存到文档库。",
			env.Name,
			graph.CountByKind(architecture.NodeKindServer),
			graph.CountByKind(architecture.NodeKindPort),
			graph.CountByKind(architecture.NodeKindContainer),
		),
		Progress:  progress,
		DocID:     doc.ID,
		DocTitle:  doc.Title,
		Intent:    "analyze_architecture",
		CreatedAt: time.Now(),
	})
	session.UpdatedAt = time.Now()
	if err := r.Sessions.Save(session); err != nil {
		r.emitError(req.RequestID, session.ID, err.Error())
		return
	}
	r.emitDone(req.RequestID, session.ID)
}

// makeArchitectureSummarizer 把 runtime 的 LLMProvider 适配成 architecture.Summarizer。
// 未配 LLM 时返回 nil，让 architecture 包退化为纯事实报告。
func (r *Runtime) makeArchitectureSummarizer() architecture.Summarizer {
	if r.LLM == nil {
		return nil
	}
	return func(messages []clients.ChatMessage) (string, error) {
		completion, err := r.LLM.ChatWithTools(messages, nil)
		if err != nil {
			return "", err
		}
		return completion.Content, nil
	}
}
