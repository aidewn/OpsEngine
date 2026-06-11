// propose 工具的 runtime 侧实现：工具循环内的工作流落盘闭包与生成任务引导段。
//
// 设计（计划书 P4）：
//   - 工具只是薄壳，解析/校验/落盘/事件全在这里，闭包捕获本轮 req 与会话指针
//   - 单轮最多成功提交 maxProposalsPerTurn 次，防模型刷资产
//   - 更新路径遵循 ApplyMode：确认模式下暂存草案（复用 P3 pending 机制）
//   - 闭包只改 session 结构体，事件即时 Emit；会话落盘由回合末尾统一完成

package runtime

import (
	"encoding/json"
	"fmt"
	"time"

	"OpsEngine/internal/agent/tools"
	"OpsEngine/internal/agent/workflow"
	"OpsEngine/internal/core"
)

// maxProposalsPerTurn 单轮会话允许成功提交草案的次数上限。
const maxProposalsPerTurn = 2

// proposalState 跟踪单轮内的提交次数，闭包间共享。
type proposalState struct{ accepted int }

// buildProposeWorkflow 返回 propose_workflow 工具的执行闭包（创建新工作流，始终直接保存）。
func (r *Runtime) buildProposeWorkflow(req Request, session *core.AISession, st *proposalState) func(string) (tools.ProposalResult, error) {
	return func(draftJSON string) (tools.ProposalResult, error) {
		if err := r.checkProposalQuota(st); err != nil {
			return tools.ProposalResult{}, err
		}
		draft, err := workflow.ParseDraft(draftJSON)
		if err != nil {
			return tools.ProposalResult{}, err
		}
		wf, err := workflow.Materialize(draft, workflow.NodeTypeChecker(r.NodeChecker))
		if err != nil {
			return tools.ProposalResult{}, err
		}
		if err := r.validateEnvRefs(wf.Nodes); err != nil {
			return tools.ProposalResult{}, err
		}
		if r.Workflows == nil {
			return tools.ProposalResult{}, fmt.Errorf("工作流存储未初始化")
		}
		if err := r.Workflows.Save(wf); err != nil {
			return tools.ProposalResult{}, err
		}
		st.accepted++
		// 进入编辑模式（含会话即时落盘），后续"继续修改"可直接路由
		if err := r.setSessionActiveArtifact(session, "workflow", wf.ID, wf.Name); err != nil {
			return tools.ProposalResult{}, err
		}
		r.Emit.Emit(Event{
			RequestID: req.RequestID, SessionID: session.ID,
			Type:         EventWorkflow,
			WorkflowID:   wf.ID,
			WorkflowName: wf.Name,
			ArtifactType: "workflow",
			ActionType:   "create",
			NodeCount:    len(wf.Nodes),
		})
		return tools.ProposalResult{
			WorkflowID:   wf.ID,
			WorkflowName: wf.Name,
			NodeCount:    len(wf.Nodes),
		}, nil
	}
}

// buildProposeWorkflowUpdate 返回 propose_update_workflow 工具的执行闭包。
// 确认模式下暂存待确认草案，否则直接保存。
func (r *Runtime) buildProposeWorkflowUpdate(req Request, session *core.AISession, st *proposalState) func(string, string) (tools.ProposalResult, error) {
	return func(workflowID, draftJSON string) (tools.ProposalResult, error) {
		if err := r.checkProposalQuota(st); err != nil {
			return tools.ProposalResult{}, err
		}
		if r.Workflows == nil {
			return tools.ProposalResult{}, fmt.Errorf("工作流存储未初始化")
		}
		existing, err := r.Workflows.Get(workflowID)
		if err != nil {
			return tools.ProposalResult{}, fmt.Errorf("工作流 %s 不存在: %w", workflowID, err)
		}
		draft, err := workflow.ParseDraft(draftJSON)
		if err != nil {
			return tools.ProposalResult{}, err
		}
		wf, err := workflow.MaterializeWorkflowUpdate(draft, existing, workflow.NodeTypeChecker(r.NodeChecker))
		if err != nil {
			return tools.ProposalResult{}, err
		}
		if err := r.validateEnvRefs(wf.Nodes); err != nil {
			return tools.ProposalResult{}, err
		}
		diff := workflow.DiffWorkflows(existing, wf)
		changeSummary := diff.Summary()
		st.accepted++

		if r.ApplyMode == ApplyModeConfirm {
			// 暂存草案：只改结构体 + 发事件；会话落盘由回合末尾统一完成
			raw, err := json.Marshal(wf)
			if err != nil {
				return tools.ProposalResult{}, fmt.Errorf("草案序列化失败: %w", err)
			}
			session.PendingDraft = &core.AIPendingDraft{
				ArtifactType:  "workflow",
				ArtifactID:    wf.ID,
				ArtifactName:  wf.Name,
				DraftJSON:     string(raw),
				BaseHash:      workflowHash(existing),
				ChangeSummary: changeSummary,
				CreatedAt:     time.Now(),
			}
			r.Emit.Emit(Event{
				RequestID: req.RequestID, SessionID: session.ID,
				Type:          EventWorkflowPending,
				WorkflowID:    wf.ID,
				WorkflowName:  wf.Name,
				ArtifactType:  "workflow",
				ActionType:    "pending",
				NodeCount:     len(wf.Nodes),
				ChangeSummary: changeSummary,
			})
			return tools.ProposalResult{
				WorkflowID:    wf.ID,
				WorkflowName:  wf.Name,
				NodeCount:     len(wf.Nodes),
				ChangeSummary: changeSummary,
				Pending:       true,
			}, nil
		}

		if err := r.Workflows.Save(wf); err != nil {
			return tools.ProposalResult{}, err
		}
		if err := r.setSessionActiveArtifact(session, "workflow", wf.ID, wf.Name); err != nil {
			return tools.ProposalResult{}, err
		}
		r.Emit.Emit(Event{
			RequestID: req.RequestID, SessionID: session.ID,
			Type:          EventWorkflow,
			WorkflowID:    wf.ID,
			WorkflowName:  wf.Name,
			ArtifactType:  "workflow",
			ActionType:    "update",
			NodeCount:     len(wf.Nodes),
			ChangeSummary: changeSummary,
		})
		return tools.ProposalResult{
			WorkflowID:    wf.ID,
			WorkflowName:  wf.Name,
			NodeCount:     len(wf.Nodes),
			ChangeSummary: changeSummary,
		}, nil
	}
}

// checkProposalQuota 限制单轮成功提交次数。
func (r *Runtime) checkProposalQuota(st *proposalState) error {
	if st.accepted >= maxProposalsPerTurn {
		return fmt.Errorf("单轮会话最多提交 %d 次草案，请先与用户确认后续修改", maxProposalsPerTurn)
	}
	return nil
}

// buildGenerationGuidance 生成"工作流编排任务"的 system 引导段。
// 节点目录不在此处全量注入——模型应通过 node_catalog 工具按需查询，节省上下文。
func buildGenerationGuidance(currentWorkflowJSON string) string {
	guidance := `【工作流编排任务】
用户本轮要求生成或修改工作流。执行要求：
1. 先用 node_catalog 查询节点 type_id 与 config schema（必填字段不可省略）；不确定的服务器路径/服务名/容器名，先用只读工具（ssh_list_dir、docker_list_containers 等）核实，禁止凭空编造
2. 设计完成后调用 propose_workflow（新建）或 propose_update_workflow（修改已有），draft_json 传完整草案 JSON
3. 工作流必须以 system_ready 节点为入口；更新时未修改的节点原样回显其 instance_id
4. 提交失败会返回具体校验错误（含节点与字段 ID），修正后重试，不要原样重发
5. 成功后用中文向用户简要总结（节点构成、关键配置、注意事项），不要把 JSON 原文贴给用户`
	if currentWorkflowJSON != "" {
		guidance += "\n\n【当前工作流 JSON】（更新基线，未修改节点请回显 instance_id）\n" + currentWorkflowJSON
	}
	return guidance
}
