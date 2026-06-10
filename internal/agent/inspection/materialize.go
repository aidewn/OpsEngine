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

	// 先把每个 Item 翻译成具体 shell 命令；安全检查在 RenderCommand 内部完成（仅 shell Kind 走黑名单，
	// 其余 Kind 命令由 Materializer 自己拼接，参数走 shellQuote 防注入）。
	commands := make([]string, len(plan.Items))
	for i, it := range plan.Items {
		cmd, err := RenderCommand(it)
		if err != nil {
			return core.WorkflowDef{}, fmt.Errorf("巡检项 %q: %w", it.Title, err)
		}
		commands[i] = cmd
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

	// 逐项追加 linux_exec_command 节点；命令文本来自上面的 commands 切片。
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
				"command":         commands[i],
				"timeout_seconds": int64(timeout),
				"fail_on_error":   false,
				// title / kind 不在节点 schema 内但保留进 config 方便前端 + 报告生成识别
				"title": strings.TrimSpace(item.Title),
				"kind":  string(item.resolveKind()),
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

// RenderCommand 把单条巡检项翻译成具体 shell 命令。
//
// 安全约束：
//   - 只有 Kind=shell 接受用户/模型传入的任意命令，走 dangerousTokens 黑名单。
//   - 其余 Kind 由 Materializer 拼模板，参数（path/container/service/...）必须走 shellQuoteArg
//     做单引号转义，杜绝 ;`$| 等元字符注入。
//   - 用户传入的 namespace / label selector 也会走 shellQuoteArg 即使它们看起来"安全"。
func RenderCommand(it Item) (string, error) {
	kind := it.resolveKind()
	switch kind {
	case ItemKindShell:
		cmd := strings.TrimSpace(it.Command)
		if err := assertSafeShellCommand(it.Title, cmd); err != nil {
			return "", err
		}
		return cmd, nil
	case ItemKindReadFile:
		if err := assertSafePath(it.Path); err != nil {
			return "", err
		}
		// head -c 65536：限制读取 64KB，避免 cat 大文件撑爆 LLM 上下文
		return fmt.Sprintf("head -c 65536 %s 2>&1", shellQuoteArg(it.Path)), nil
	case ItemKindReadLog:
		if err := assertSafePath(it.Path); err != nil {
			return "", err
		}
		lines := it.TailLines
		if lines <= 0 {
			lines = 100
		}
		if lines > 1000 {
			lines = 1000
		}
		return fmt.Sprintf("tail -n %d %s 2>&1", lines, shellQuoteArg(it.Path)), nil
	case ItemKindFindFiles:
		if err := assertSafePath(it.Root); err != nil {
			return "", err
		}
		pattern := strings.TrimSpace(it.Pattern)
		if pattern == "" {
			pattern = "*"
		}
		if err := assertSafeArg(pattern); err != nil {
			return "", fmt.Errorf("pattern: %w", err)
		}
		// -maxdepth 5 + head -n 200 防止深目录或海量结果拖垮巡检
		return fmt.Sprintf("find %s -maxdepth 5 -name %s 2>/dev/null | head -n 200",
			shellQuoteArg(it.Root), shellQuoteArg(pattern)), nil
	case ItemKindDockerList:
		// 含 stopped；输出 "name\timage\tstatus"，便于阅读
		return "docker ps -a --format '{{.Names}}\\t{{.Image}}\\t{{.Status}}' 2>&1", nil
	case ItemKindDockerLogs:
		if err := assertSafeArg(it.Container); err != nil {
			return "", fmt.Errorf("container: %w", err)
		}
		lines := it.TailLines
		if lines <= 0 {
			lines = 200
		}
		if lines > 1000 {
			lines = 1000
		}
		return fmt.Sprintf("docker logs --tail %d %s 2>&1",
			lines, shellQuoteArg(it.Container)), nil
	case ItemKindK8sPods:
		var b strings.Builder
		b.WriteString("kubectl get pods")
		if ns := strings.TrimSpace(it.Namespace); ns != "" {
			if err := assertSafeArg(ns); err != nil {
				return "", fmt.Errorf("namespace: %w", err)
			}
			b.WriteString(" -n ")
			b.WriteString(shellQuoteArg(ns))
		}
		if sel := strings.TrimSpace(it.LabelSelector); sel != "" {
			// label selector 允许 key=value,key2=value2 形式；ascii + 等号逗号是安全的
			if err := assertSafeArg(sel); err != nil {
				return "", fmt.Errorf("label_selector: %w", err)
			}
			b.WriteString(" -l ")
			b.WriteString(shellQuoteArg(sel))
		}
		b.WriteString(" -o wide 2>&1")
		return b.String(), nil
	case ItemKindK8sDescribe:
		// Workload 形如 "Deployment/nginx"。已在 Validate 中确认含 "/"。
		// 把它直接放入 describe 参数：kubectl describe deployment/nginx [-n ns]
		workload := strings.TrimSpace(it.Workload)
		if err := assertSafeArg(workload); err != nil {
			return "", fmt.Errorf("workload: %w", err)
		}
		var b strings.Builder
		b.WriteString("kubectl describe ")
		b.WriteString(shellQuoteArg(workload))
		if ns := strings.TrimSpace(it.Namespace); ns != "" {
			if err := assertSafeArg(ns); err != nil {
				return "", fmt.Errorf("namespace: %w", err)
			}
			b.WriteString(" -n ")
			b.WriteString(shellQuoteArg(ns))
		}
		b.WriteString(" 2>&1")
		return b.String(), nil
	case ItemKindSystemd:
		if err := assertSafeArg(it.Service); err != nil {
			return "", fmt.Errorf("service: %w", err)
		}
		// --no-pager 避免 less 阻塞 SSH 会话
		return fmt.Sprintf("systemctl status %s --no-pager 2>&1", shellQuoteArg(it.Service)), nil
	case ItemKindPortListen:
		// 优先 ss（现代发行版），缺则 netstat 兜底
		return "(ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null) | head -n 80", nil
	default:
		return "", fmt.Errorf("未知 Kind: %s", kind)
	}
}

// assertSafeShellCommand 校验任意 shell 命令是否命中危险黑名单。
func assertSafeShellCommand(title, cmd string) error {
	lower := strings.ToLower(cmd)
	for _, token := range dangerousTokens {
		if strings.Contains(lower, token) {
			return fmt.Errorf("命令命中危险禁令 %q（%s）", token, title)
		}
	}
	return nil
}

// assertSafePath 校验 LLM 传入路径：必须绝对路径、不含 shell 元字符。
func assertSafePath(path string) error {
	p := strings.TrimSpace(path)
	if p == "" {
		return errors.New("path 不能为空")
	}
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("path 必须是绝对路径: %q", p)
	}
	return assertSafeArg(p)
}

// assertSafeArg 校验参数不含 shell 元字符。
// 后续走 shellQuoteArg 单引号包裹，理论上能兜住所有元字符，但 args 内含单引号会被分段，
// 这里直接拒绝单引号 + 控制字符，避免边界情况。
func assertSafeArg(s string) error {
	if strings.ContainsAny(s, "`$|&;\n\r<>\\") {
		return fmt.Errorf("参数含非法字符: %q", s)
	}
	if strings.Contains(s, "'") {
		return fmt.Errorf("参数不允许包含单引号: %q", s)
	}
	return nil
}

// shellQuoteArg 把参数用单引号包裹，单引号本身已经在 assertSafeArg 拒绝，无需再转义。
func shellQuoteArg(s string) string {
	return "'" + s + "'"
}
