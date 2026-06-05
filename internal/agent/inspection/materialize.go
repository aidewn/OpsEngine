// 把 Plan 翻译成可保存、可执行的 core.WorkflowDef。
//
// 工作流形状（线性链）：
//
//   system_ready ──exec──▶ env_connect_ssh ──exec──▶ linux_exec_command[0] ──exec──▶ linux_exec_command[1] ──exec──▶ ...
//                                │                          ▲                              ▲
//                                └──── client (SSH) ────────┘                              │
//                                └──── client (SSH) ────────────────────────────────────── ┘
//
// 关键约束（参见 internal/engine/validate.go）：
//   - exec_out 每个最多 1 条出边 → 每个节点的 exec_out 只连一个下游
//   - 数据输入端口最多 1 条入边 → 每个 linux_exec_command.client 只接 1 条 SSH
//   - 数据输出端口可以扇出 → env_connect_ssh.client 可以同时喂多个 linux_exec_command

package inspection

import (
	"errors"
	"fmt"
	"strings"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	"github.com/google/uuid"
)

// 工作流画布坐标常量。X 步长选 240px 让节点之间能看见连线，不挤在一起。
const (
	canvasOriginX float64 = 80
	canvasOriginY float64 = 160
	canvasStepX   float64 = 240
)

// 默认命令超时时间。10 秒足以覆盖典型只读巡检命令（uname/df/ss/ps 等）。
const defaultItemTimeoutSeconds = 10

// dangerousTokens 是命令安全黑名单。任何巡检项命中其中之一一律拒绝落地。
// 这里使用大小写无关的子串匹配（而不是完整 shell parsing），
// 因为巡检场景下宁可误杀也不能让破坏性命令进入工作流。
var dangerousTokens = []string{
	"rm -rf", "rm -fr", "mkfs", "shutdown", "reboot", "halt", "poweroff",
	"dd if=", ":(){:|:&};:", "> /dev/sd", ">/dev/sd",
}

// SSHTargetOption 是多 SSH 场景下返回给 Runtime/前端的候选项。
type SSHTargetOption struct {
	ID   string
	Name string
}

// SSHTargetSelectionError 表示环境内存在多个 SSH 配置，需要用户选择目标。
type SSHTargetSelectionError struct {
	EnvironmentID string
	Candidates    []SSHTargetOption
}

// Error 返回用户可读的追问文案。
func (e *SSHTargetSelectionError) Error() string {
	return fmt.Sprintf("环境 %s 内有多个 SSH 配置，请明确指定要巡检的目标", e.EnvironmentID)
}

// PickSSHTarget 根据用户偏好和环境内 SSH 数量选定巡检目标。
//
// 决策顺序：
//  1. 用户显式指定 configID（来自会话 ConfigID 或本轮请求）→ 校验是 SSH 且存在
//  2. 否则环境内**唯一**一台 SSH → 自动选定
//  3. 否则返回明确错误，由 runtime 回显给用户追问
//
// 这是文档 P1.5 / P5 中"环境级会话+巡检 MVP"的关键决策点：
// 不让模型自由挑 SSH 配置，避免环境复杂时跑错机器。
func PickSSHTarget(env core.EnvironmentDef, preferredConfigID string) (string, error) {
	preferred := strings.TrimSpace(preferredConfigID)
	if preferred != "" {
		for _, c := range env.Configs {
			if c.ID != preferred {
				continue
			}
			if c.Kind != core.EnvConfigKindSSH {
				return "", fmt.Errorf("指定配置 %s 不是 SSH 配置（kind=%s）", preferred, c.Kind)
			}
			return preferred, nil
		}
		return "", fmt.Errorf("环境 %s 中未找到 SSH 配置 %s", env.ID, preferred)
	}
	candidates := []SSHTargetOption{}
	for _, c := range env.Configs {
		if c.Kind != core.EnvConfigKindSSH {
			continue
		}
		candidates = append(candidates, SSHTargetOption{ID: c.ID, Name: c.Name})
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("环境 %s 内没有 SSH 配置，无法生成巡检工作流", env.ID)
	}
	if len(candidates) > 1 {
		return "", &SSHTargetSelectionError{EnvironmentID: env.ID, Candidates: candidates}
	}
	return candidates[0].ID, nil
}

// Materialize 把 Plan 翻译成 core.WorkflowDef。
//   - environmentID / configID 必须是调用方已经绑定到当前会话的 SSH 配置；这里只写进 env_connect_ssh.config。
//   - 命令安全黑名单一旦命中直接报错，调用方应把错误回显给用户。
//   - 工作流最终跑一次 engine.ValidateWorkflow 保证保存后的可执行性。
func Materialize(plan Plan, environmentID, configID string) (core.WorkflowDef, error) {
	if strings.TrimSpace(environmentID) == "" || strings.TrimSpace(configID) == "" {
		return core.WorkflowDef{}, errors.New("巡检工作流需要先绑定 SSH 环境与配置")
	}
	if err := plan.Validate(); err != nil {
		return core.WorkflowDef{}, err
	}
	if err := assertSafeCommands(plan.Items); err != nil {
		return core.WorkflowDef{}, err
	}

	readyID := uuid.New().String()
	sshID := uuid.New().String()
	nodes := []core.NodeInstance{
		{
			InstanceID: readyID,
			TypeID:     "system_ready",
			Config:     map[string]any{},
			Position:   core.Position{X: canvasOriginX, Y: canvasOriginY},
		},
		{
			InstanceID: sshID,
			TypeID:     "env_connect_ssh",
			Config: map[string]any{
				"environment_id": environmentID,
				"config_id":      configID,
			},
			Position: core.Position{X: canvasOriginX + canvasStepX, Y: canvasOriginY},
		},
	}
	edges := []core.EdgeConfig{
		{
			From: core.PortRef{Node: readyID, Port: "exec_out"},
			To:   core.PortRef{Node: sshID, Port: "exec_in"},
		},
	}

	// 逐项追加 linux_exec_command 节点。
	// prevExecNode 跟踪当前 exec 链尾，让 N 个命令串行执行。
	prevExecNode, prevExecPort := sshID, "exec_out"
	for i, item := range plan.Items {
		cmdID := uuid.New().String()
		timeout := item.TimeoutSeconds
		if timeout <= 0 {
			timeout = defaultItemTimeoutSeconds
		}
		nodes = append(nodes, core.NodeInstance{
			InstanceID: cmdID,
			TypeID:     "linux_exec_command",
			Config: map[string]any{
				"command":         strings.TrimSpace(item.Command),
				"timeout_seconds": int64(timeout),
				// fail_on_error=false 让单项失败不中断后续巡检——巡检的本质是"尽量多采集"。
				"fail_on_error": false,
				// title 字段不在节点 schema 内，但保留进 config 方便前端展示。
				"title": strings.TrimSpace(item.Title),
			},
			Position: core.Position{
				X: canvasOriginX + canvasStepX*float64(2+i),
				Y: canvasOriginY,
			},
		})
		// exec 串：上一个节点的 exec_out → 当前节点的 exec_in。
		edges = append(edges, core.EdgeConfig{
			From: core.PortRef{Node: prevExecNode, Port: prevExecPort},
			To:   core.PortRef{Node: cmdID, Port: "exec_in"},
		})
		// SSH client 扇出：env_connect_ssh.client → 每个 linux_exec_command.client。
		edges = append(edges, core.EdgeConfig{
			From: core.PortRef{Node: sshID, Port: "client"},
			To:   core.PortRef{Node: cmdID, Port: "client"},
		})
		prevExecNode, prevExecPort = cmdID, "exec_out"
	}

	name := strings.TrimSpace(plan.Name)
	if name == "" {
		name = "服务器巡检工作流"
	}
	wf := core.WorkflowDef{
		ID:          uuid.New().String(),
		Name:        name,
		Description: strings.TrimSpace(plan.Description),
		Variables:   []core.VariableDef{},
		Nodes:       nodes,
		Edges:       edges,
	}
	if err := engine.ValidateWorkflow(wf); err != nil {
		return core.WorkflowDef{}, fmt.Errorf("巡检工作流校验失败: %w", err)
	}
	return wf, nil
}

// assertSafeCommands 扫描所有 item.Command，命中黑名单立即拒绝。
// 黑名单不可能穷尽所有危险操作，但能拦下最常见的破坏性命令。
// 后续 P7 Agent Tool Registry 落地后，这一层应升级为统一的权限校验。
func assertSafeCommands(items []Item) error {
	for _, it := range items {
		lower := strings.ToLower(it.Command)
		for _, token := range dangerousTokens {
			if strings.Contains(lower, token) {
				return fmt.Errorf("巡检项 %q 命中危险命令禁令 %q", it.Title, token)
			}
		}
	}
	return nil
}
