// linux_download_file 节点：通过远程 SSH 把 URL 下载到目标路径
// 自动检测远程主机有没有 curl，没有就回退 wget；都没有则失败
//
// 设计要点：
//   - 一次 SSH session 完成 探测+下载，避免两次 round trip
//   - 通过 stderr 的 sentinel 行 "__OPSENGINE_TOOL__=xxx" 解析实际使用的工具
//   - HTTP 4xx/5xx 通过 curl -f / wget 默认行为产生非零 exit_code → return error
//   - 失败时仍然输出 tool_used / exit_code，便于排查（与 linux_exec_command 风格一致）

package linux_download_file

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	"golang.org/x/crypto/ssh"
)

const (
	defaultTimeoutSeconds = 60
	toolSentinelPrefix    = "__OPSENGINE_TOOL__="
)

func init() { engine.Register(&Node{}) }

// Node linux_download_file 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	minTimeout, maxTimeout := int64(1), int64(3600)
	return core.NodeTypeDef{
		TypeID:      "linux_download_file",
		DisplayName: "Linux 下载文件",
		Category:    "remote",
		NodeKind:    core.NodeKindAction,
		Icon:        "⬇",
		Description: "通过 SSH 在远程主机用 curl 或 wget 把 URL 下载到指定路径，自动选择可用工具",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "client", Label: "SSH", PortType: core.PortTypeLinuxSsh, Required: true},
			{ID: "url", Label: "URL", PortType: core.PortTypeString},
			{ID: "dest_path", Label: "目标路径", PortType: core.PortTypeString},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "success", Label: "成功", PortType: core.PortTypeBool},
			{ID: "exit_code", Label: "退出码", PortType: core.PortTypeInt},
			{ID: "dest_path", Label: "实际路径", PortType: core.PortTypeString},
			{ID: "tool_used", Label: "工具", PortType: core.PortTypeString},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "url", Label: "URL", Required: true,
				Placeholder: "http://mirror.example.com/centos.repo"},
			{Type: "text", ID: "dest_path", Label: "目标路径", Required: true,
				Placeholder: "/etc/yum.repos.d/example.repo"},
			{Type: "number", ID: "timeout_seconds", Label: "超时（秒）",
				Min: &minTimeout, Max: &maxTimeout, Default: int64(defaultTimeoutSeconds)},
			{Type: "toggle", ID: "follow_redirects", Label: "跟随重定向", Default: true},
			{Type: "toggle", ID: "insecure", Label: "跳过证书校验", Default: false},
			{Type: "toggle", ID: "ensure_parent_dir", Label: "自动创建父目录", Default: true},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 在远程主机执行下载
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	sshClient, err := inputClient(ctx)
	if err != nil {
		return nil, err
	}

	url := strings.TrimSpace(stringInput(ctx, "url"))
	if url == "" {
		url = strings.TrimSpace(ctx.ConfigString("url"))
	}
	if url == "" {
		return nil, fmt.Errorf("linux_download_file 节点的 url 未配置")
	}

	dest := strings.TrimSpace(stringInput(ctx, "dest_path"))
	if dest == "" {
		dest = strings.TrimSpace(ctx.ConfigString("dest_path"))
	}
	if dest == "" {
		return nil, fmt.Errorf("linux_download_file 节点的 dest_path 未配置")
	}

	timeoutSeconds := ctx.ConfigInt("timeout_seconds")
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultTimeoutSeconds
	}

	script := buildDownloadScript(downloadOpts{
		URL:             url,
		Dest:            dest,
		TimeoutSeconds:  timeoutSeconds,
		FollowRedirects: ctx.ConfigBool("follow_redirects"),
		Insecure:        ctx.ConfigBool("insecure"),
		EnsureParentDir: ctx.ConfigBool("ensure_parent_dir"),
	})

	client := sshClient.Client()
	if client == nil {
		return nil, fmt.Errorf("LinuxSshConnection 未连接或已不可用")
	}
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("创建 SSH session 失败: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	session.Stdin = strings.NewReader(script)

	ctx.Info("远程下载: %s -> %s", url, dest)
	runErr := runWithTimeout(ctx, session, "bash -s", time.Duration(timeoutSeconds)*time.Second)

	exitCode := exitCodeOf(runErr)
	tool := parseToolUsed(stderr.String())

	outputs := engine.Outputs{
		"success":   runErr == nil,
		"exit_code": int64(exitCode),
		"dest_path": dest,
		"tool_used": tool,
	}

	stderrText := strings.TrimSpace(stripToolSentinel(stderr.String()))
	if runErr != nil {
		ctx.Error("下载失败 exit_code=%d", exitCode)
		if stderrText != "" {
			ctx.Error("stderr: %s", stderrText)
		}
		return outputs, fmt.Errorf("下载失败 exit_code=%d: %w", exitCode, runErr)
	}

	if tool != "" {
		ctx.Info("下载成功（工具=%s）", tool)
	} else {
		ctx.Info("下载成功")
	}
	if stderrText != "" {
		ctx.Warn("stderr: %s", stderrText)
	}
	return outputs, nil
}

// downloadOpts 拼脚本时用到的全部参数
type downloadOpts struct {
	URL             string
	Dest            string
	TimeoutSeconds  int64
	FollowRedirects bool
	Insecure        bool
	EnsureParentDir bool
}

// buildDownloadScript 拼接 探测+下载 一次性 bash 脚本
// 通过 stdin 喂给远程的 `bash -s`，无需在远程留临时文件
func buildDownloadScript(opts downloadOpts) string {
	var b strings.Builder
	b.WriteString("set -e\n")
	if opts.EnsureParentDir {
		b.WriteString("mkdir -p \"$(dirname ")
		b.WriteString(shellQuote(opts.Dest))
		b.WriteString(")\"\n")
	}

	curlOpts := "-fsS"
	if opts.FollowRedirects {
		curlOpts += "L"
	}
	if opts.Insecure {
		curlOpts += "k"
	}

	wgetInsecure := ""
	if opts.Insecure {
		wgetInsecure = " --no-check-certificate"
	}

	b.WriteString("if command -v curl >/dev/null 2>&1; then\n")
	b.WriteString("  printf '")
	b.WriteString(toolSentinelPrefix)
	b.WriteString("curl\\n' >&2\n")
	fmt.Fprintf(&b, "  curl %s --max-time %d -o %s %s\n",
		curlOpts, opts.TimeoutSeconds, shellQuote(opts.Dest), shellQuote(opts.URL))
	b.WriteString("elif command -v wget >/dev/null 2>&1; then\n")
	b.WriteString("  printf '")
	b.WriteString(toolSentinelPrefix)
	b.WriteString("wget\\n' >&2\n")
	fmt.Fprintf(&b, "  wget -q%s --timeout=%d -O %s %s\n",
		wgetInsecure, opts.TimeoutSeconds, shellQuote(opts.Dest), shellQuote(opts.URL))
	b.WriteString("else\n")
	b.WriteString("  echo \"neither curl nor wget available\" >&2\n")
	b.WriteString("  exit 127\n")
	b.WriteString("fi\n")
	return b.String()
}

// shellQuote 用 POSIX shell 的单引号语义包裹任意字符串，原文里的 ' 用 '\\'' 转义
// 这样无论 URL / dest 含什么字符（空格、$、`），都不会被 shell 解释
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// parseToolUsed 从 stderr 文本里找 sentinel 行得到实际使用的工具
// 找不到返回空串（脚本可能在 sentinel 之前就 fail）
func parseToolUsed(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, toolSentinelPrefix) {
			return strings.TrimPrefix(line, toolSentinelPrefix)
		}
	}
	return ""
}

// stripToolSentinel 从 stderr 文本里移除 sentinel 行，便于日志展示真正的错误信息
func stripToolSentinel(stderr string) string {
	var keep []string
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), toolSentinelPrefix) {
			continue
		}
		keep = append(keep, line)
	}
	return strings.Join(keep, "\n")
}

// inputClient 读取必填的 SSH 连接句柄
func inputClient(ctx engine.ExecContext) (*clients.LinuxSshClient, error) {
	value, ok := ctx.Input("client")
	if !ok || value == nil {
		return nil, fmt.Errorf("linux_download_file 节点缺少 client 输入")
	}
	c, ok := value.(*clients.LinuxSshClient)
	if !ok {
		return nil, fmt.Errorf("client 输入类型不是 LinuxSshConnection")
	}
	return c, nil
}

// stringInput 读字符串 input 端口；缺失或类型不匹配返回空串
func stringInput(ctx engine.ExecContext, portID string) string {
	value, ok := ctx.Input(portID)
	if !ok || value == nil {
		return ""
	}
	s, _ := value.(string)
	return s
}

// runWithTimeout 复用 linux_exec_command 的执行模式：节点超时 + 工作流取消 双路 select
func runWithTimeout(ctx engine.ExecContext, session *ssh.Session, command string, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- session.Run(command)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case err := <-done:
		return err
	case <-ctx.Context().Done():
		_ = session.Close()
		return ctx.Context().Err()
	case <-timer.C:
		_ = session.Close()
		return fmt.Errorf("下载超时: %s", timeout)
	}
}

// exitCodeOf 从 SSH 错误中提取退出码；非命令退出错误统一返回 -1
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitStatus()
	}
	return -1
}
