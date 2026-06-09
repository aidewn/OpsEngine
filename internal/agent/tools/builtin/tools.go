// 内置工具集合：v1 全部为只读类型。
// 每个工具的 Spec.Description 直接影响模型的工具选择准确率，写得越精确越好。

package builtin

import (
	"fmt"
	"strings"

	agentctx "OpsEngine/internal/agent/context"
	"OpsEngine/internal/agent/tools"
)

// Register 把内置工具注册到给定 Registry。
// 调用方（ai.go）在 App 启动时调用一次即可。
func Register(reg *tools.Registry) error {
	for _, t := range []tools.Tool{
		EnvInventory{},
		SSHInspect{},
		SSHListDir{},
		SSHReadLog{},
		SSHProcessList{},
	} {
		if err := reg.Register(t); err != nil {
			return err
		}
	}
	return nil
}

// ── env_inventory ─────────────────────────────────────────────

// EnvInventory 返回当前环境的资产清单（脱敏）。不需要 SSH 连接。
type EnvInventory struct{}

func (EnvInventory) Spec() tools.Spec {
	return tools.Spec{
		Name:        "env_inventory",
		Description: "查看当前会话所在环境的资产清单：SSH/Docker/K8s 等配置的脱敏摘要。用于先了解可操作的资产范围。",
		Tier:        tools.TierRead,
		Params:      map[string]tools.ParamSpec{},
	}
}

func (EnvInventory) Execute(ctx tools.ToolContext, _ map[string]any) (tools.Result, error) {
	if ctx.EnvLookup == nil {
		return tools.Result{}, fmt.Errorf("环境查询未注入")
	}
	env, err := ctx.EnvLookup(ctx.EnvironmentID)
	if err != nil {
		return tools.Result{}, err
	}
	text := agentctx.BuildInventory(env).RenderText()
	return tools.Result{
		Output:         tools.TruncateOutput(text),
		DisplaySummary: fmt.Sprintf("环境 %s（%d 个配置）", env.Name, len(env.Configs)),
	}, nil
}

// ── ssh_inspect ───────────────────────────────────────────────

// SSHInspect 跑一次只读巡检脚本，返回服务器基础状态。
// 等同于 chat 流程中的 SSH prefetch，但作为工具明确暴露给 LLM。
type SSHInspect struct{}

func (SSHInspect) Spec() tools.Spec {
	return tools.Spec{
		Name: "ssh_inspect",
		Description: "采集目标 SSH 服务器的基础状态：系统信息、负载、内存、磁盘、CPU 前 10 进程、网卡、监听端口。" +
			"在需要快速了解机器现状时调用，例如用户问『机器情况』『有问题吗』。",
		Tier:   tools.TierRead,
		Params: map[string]tools.ParamSpec{},
	}
}

func (SSHInspect) Execute(ctx tools.ToolContext, _ map[string]any) (tools.Result, error) {
	_, fields, envName, err := pickSSHTargetForTool(ctx)
	if err != nil {
		return tools.Result{}, err
	}
	out, err := dialAndRun(fields, inspectScript)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{
		Output:         out,
		DisplaySummary: fmt.Sprintf("ssh_inspect @ %s", envName),
	}, nil
}

// inspectScript 与 agentctx.Prefetch 中的脚本基本一致，集中在工具层方便迭代。
const inspectScript = `set +e
echo "## 系统信息"; uname -a 2>/dev/null
(cat /etc/os-release 2>/dev/null || cat /etc/issue 2>/dev/null) | head -5
echo "## 负载"; uptime 2>/dev/null
echo "## 内存"; (free -h 2>/dev/null) || (head -5 /proc/meminfo 2>/dev/null)
echo "## 磁盘"; df -hT 2>/dev/null
echo "## CPU 前 10 进程"; ps -eo pid,user,pcpu,pmem,comm --sort=-pcpu 2>/dev/null | head -11
echo "## 网卡"; (ip -br addr 2>/dev/null) || (ifconfig 2>/dev/null | head -10)
echo "## 监听端口"; (ss -tln 2>/dev/null | head -20) || (netstat -tln 2>/dev/null | head -20)
`

// ── ssh_list_dir ──────────────────────────────────────────────

// SSHListDir 列目录。等同于 ls -la <path>。
type SSHListDir struct{}

func (SSHListDir) Spec() tools.Spec {
	return tools.Spec{
		Name:        "ssh_list_dir",
		Description: "列出目标 SSH 服务器上的目录内容（ls -la）。用于查看文件结构、配置目录、日志目录。",
		Tier:        tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"path": {Type: "string", Description: "绝对路径，例如 /etc/nginx 或 /var/log。", Required: true},
		},
	}
}

func (SSHListDir) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	path, err := argString(args, "path")
	if err != nil {
		return tools.Result{}, err
	}
	if !looksLikeSafePath(path) {
		return tools.Result{}, fmt.Errorf("path 必须是绝对路径，不含 shell 特殊字符")
	}
	_, fields, _, err := pickSSHTargetForTool(ctx)
	if err != nil {
		return tools.Result{}, err
	}
	out, err := dialAndRun(fields, fmt.Sprintf("ls -la --color=never %s 2>&1", shellQuote(path)))
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{Output: out, DisplaySummary: "ssh_list_dir " + path}, nil
}

// ── ssh_read_log ──────────────────────────────────────────────

// SSHReadLog 读日志尾部。等同于 tail -n N <path>。
type SSHReadLog struct{}

func (SSHReadLog) Spec() tools.Spec {
	return tools.Spec{
		Name:        "ssh_read_log",
		Description: "读取目标 SSH 服务器上日志文件的尾部（tail -n）。用于查看最近的错误、访问、应用日志。",
		Tier:        tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"path":  {Type: "string", Description: "日志文件绝对路径。", Required: true},
			"lines": {Type: "integer", Description: "读取尾部多少行，默认 100，上限 500。", Required: false, Default: 100},
		},
	}
}

func (SSHReadLog) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	path, err := argString(args, "path")
	if err != nil {
		return tools.Result{}, err
	}
	if !looksLikeSafePath(path) {
		return tools.Result{}, fmt.Errorf("path 必须是绝对路径，不含 shell 特殊字符")
	}
	lines := argInt(args, "lines", 100)
	if lines < 1 {
		lines = 100
	}
	if lines > 500 {
		lines = 500
	}
	_, fields, _, err := pickSSHTargetForTool(ctx)
	if err != nil {
		return tools.Result{}, err
	}
	out, err := dialAndRun(fields, fmt.Sprintf("tail -n %d %s 2>&1", lines, shellQuote(path)))
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{Output: out, DisplaySummary: fmt.Sprintf("ssh_read_log %s (%d 行)", path, lines)}, nil
}

// ── ssh_process_list ──────────────────────────────────────────

// SSHProcessList 列 CPU 占用前 N 进程。
type SSHProcessList struct{}

func (SSHProcessList) Spec() tools.Spec {
	return tools.Spec{
		Name:        "ssh_process_list",
		Description: "列出目标 SSH 服务器上按 CPU 占用排序的前 N 个进程（ps + sort）。用于诊断高 CPU、定位异常进程。",
		Tier:        tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"top": {Type: "integer", Description: "返回前几个进程，默认 10，上限 50。", Required: false, Default: 10},
		},
	}
}

func (SSHProcessList) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	top := argInt(args, "top", 10)
	if top < 1 {
		top = 10
	}
	if top > 50 {
		top = 50
	}
	_, fields, _, err := pickSSHTargetForTool(ctx)
	if err != nil {
		return tools.Result{}, err
	}
	out, err := dialAndRun(fields, fmt.Sprintf("ps -eo pid,user,pcpu,pmem,comm --sort=-pcpu 2>/dev/null | head -%d", top+1))
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{Output: out, DisplaySummary: fmt.Sprintf("ssh_process_list top %d", top)}, nil
}

// ── path 安全检查 ─────────────────────────────────────────────

// looksLikeSafePath 验证 LLM 传入的路径不会被 shell 解析成奇怪东西。
// 必须是绝对路径，不含分号 / 反引号 / $ / | / & / 换行等元字符。
func looksLikeSafePath(path string) bool {
	if !strings.HasPrefix(path, "/") {
		return false
	}
	if strings.ContainsAny(path, ";`$|&\n\r<>") {
		return false
	}
	return true
}

// shellQuote 把路径用单引号包起来，单引号本身在 shell 字符串里不可转义，
// 所以直接拒绝带单引号的路径（looksLikeSafePath 已限定字符集，理论不会到这里）。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
