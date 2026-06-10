// 会话级 artifact 编辑模式：对标 Claude Code 的「正在编辑某资产」上下文。

package runtime

import (
	"fmt"
	"strings"

	"OpsEngine/internal/agent/intent"
	"OpsEngine/internal/core"
)

// applyArtifactRouting 在 auto 模式下，若会话处于编辑模式则将普通 chat 路由为 update。
func applyArtifactRouting(req *Request, session core.AISession, decision *intent.Result) {
	if req.Operation != "" && req.Operation != "auto" {
		return
	}
	if session.ActiveArtifactID == "" || session.ActiveArtifactType == "" {
		return
	}
	if isExplicitNewAsset(req.Message) {
		return
	}
	switch session.ActiveArtifactType {
	case "workflow":
		if decision.Kind == intent.KindChat || decision.Kind == intent.KindGenerateWorkflow {
			decision.Kind = intent.KindUpdateWorkflow
			decision.Reason = "会话工作流编辑模式"
			if req.ArtifactID == "" {
				req.ArtifactID = session.ActiveArtifactID
			}
			if req.ArtifactType == "" {
				req.ArtifactType = "workflow"
			}
		}
	case "assemble":
		if decision.Kind == intent.KindChat || decision.Kind == intent.KindCreateAssemble {
			decision.Kind = intent.KindUpdateAssemble
			decision.Reason = "会话集合编辑模式"
			if req.ArtifactID == "" {
				req.ArtifactID = session.ActiveArtifactID
			}
			if req.ArtifactType == "" {
				req.ArtifactType = "assemble"
			}
		}
	}
}

// isExplicitNewAsset 判断用户是否明确要求新建资产（应退出编辑模式）。
func isExplicitNewAsset(message string) bool {
	text := strings.ToLower(strings.TrimSpace(message))
	keywords := []string{
		"新建", "重新生成", "另存", "创建新", "新的工作流", "新的集合",
		"不要改", "别改", "换一个",
		"create new", "new workflow", "new assemble", "start over",
	}
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// setSessionActiveArtifact 写入会话编辑模式并持久化。
func (r *Runtime) setSessionActiveArtifact(session *core.AISession, artifactType, id, name string) error {
	session.ActiveArtifactType = artifactType
	session.ActiveArtifactID = id
	session.ActiveArtifactName = name
	return r.Sessions.Save(*session)
}

// clearSessionActiveArtifact 清除会话编辑模式。
func (r *Runtime) clearSessionActiveArtifact(session *core.AISession) error {
	session.ActiveArtifactType = ""
	session.ActiveArtifactID = ""
	session.ActiveArtifactName = ""
	return r.Sessions.Save(*session)
}

// workflowChangeSummary 生成工作流更新摘要。
func workflowChangeSummary(before, after int) string {
	if before == after {
		return fmt.Sprintf("节点数 %d（结构微调）", after)
	}
	return fmt.Sprintf("节点 %d → %d", before, after)
}
