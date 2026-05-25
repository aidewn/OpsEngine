// linux_exec_script 节点：在远程主机执行多行内联 shell 脚本
//
// 与 linux_exec_command 的区别：
//   - 命令拼接：linux_exec_command 把整行命令拼到 `bash -c "..."`，多行 quoting 痛苦
//   - 本节点把脚本内容通过 ssh.Session.Stdin 喂给 `<interpreter> -s`，原样多行
//   - 适合 yum/apt 安装链、批量配置等多步运维脚本
//
// 设计要点：
//   - 仅支持内联脚本（textarea / script input）；远程已有脚本路径用 linux_exec_command 即可
//   - fail_on_error 默认开（在脚本前注入 set -e），符合多数运维场景预期
//   - 解释器 select bash / sh，不含 python（避免范围蔓延）

package linux_exec_script

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

const defaultTimeoutSeconds = 300

func init() { engine.Register(&Node{}) }

// Node linux_exec_script 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	minTimeout, maxTimeout := int64(1), int64(3600)
	return core.NodeTypeDef{
		TypeID:      "linux_exec_script",
		DisplayName: "Linux 执行脚本",
		Category:    "remote",
		NodeKind:    core.NodeKindAction,
		Icon:        "📜",
		Description: "在远程主机执行多行 shell 脚本（通过 SSH stdin 喂给 bash -s）",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "client", Label: "SSH", PortType: core.PortTypeLinuxSsh, Required: true},
			{ID: "script", Label: "脚本", PortType: core.PortTypeString},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "success", Label: "成功", PortType: core.PortTypeBool},
			{ID: "exit_code", Label: "退出码", PortType: core.PortTypeInt},
			{ID: "stdout", Label: "stdout", PortType: core.PortTypeString},
			{ID: "stderr", Label: "stderr", PortType: core.PortTypeString},
			{ID: "combined_output", Label: "输出", PortType: core.PortTypeString},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "textarea", ID: "default_script", Label: "脚本内容", Required: true,
				Placeholder: "yum clean all\nyum makecache"},
			{Type: "select", ID: "interpreter", Label: "解释器",
				Options: []string{"bash", "sh"}, Default: "bash"},
			{Type: "toggle", ID: "fail_on_error", Label: "首次失败即停 (set -e)", Default: true},
			{Type: "number", ID: "timeout_seconds", Label: "超时（秒）",
				Min: &minTimeout, Max: &maxTimeout, Default: int64(defaultTimeoutSeconds)},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 通过 stdin 把脚本内容喂给远程 `<interpreter> -s`
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	sshClient, err := inputClient(ctx)
	if err != nil {
		return nil, err
	}

	rawScript := scriptText(ctx)
	if rawScript == "" {
		return nil, fmt.Errorf("linux_exec_script 节点的 script 未配置")
	}

	interpreter := resolveInterpreter(ctx.ConfigString("interpreter"))
	timeoutSeconds := ctx.ConfigInt("timeout_seconds")
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultTimeoutSeconds
	}

	finalScript := rawScript
	if ctx.ConfigBool("fail_on_error") {
		finalScript = "set -e\n" + finalScript
	}

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
	session.Stdin = strings.NewReader(finalScript)

	command := interpreter + " -s"
	ctx.Info("执行远程脚本（%s, %d 字节）", interpreter, len(finalScript))
	runErr := runWithTimeout(ctx, session, command, time.Duration(timeoutSeconds)*time.Second)

	stdoutText := stdout.String()
	stderrText := stderr.String()
	combined := stdoutText + stderrText
	exitCode := exitCodeOf(runErr)

	outputs := engine.Outputs{
		"success":         runErr == nil,
		"exit_code":       int64(exitCode),
		"stdout":          stdoutText,
		"stderr":          stderrText,
		"combined_output": combined,
	}

	if runErr != nil {
		ctx.Error("脚本执行失败 exit_code=%d", exitCode)
		if trimmed := strings.TrimSpace(stderrText); trimmed != "" {
			ctx.Error("stderr: %s", trimmed)
		}
		if trimmed := strings.TrimSpace(stdoutText); trimmed != "" {
			ctx.Info("stdout: %s", trimmed)
		}
		return outputs, fmt.Errorf("脚本执行失败 exit_code=%d: %w", exitCode, runErr)
	}

	ctx.Info("脚本执行成功")
	if trimmed := strings.TrimSpace(stdoutText); trimmed != "" {
		ctx.Info("stdout: %s", trimmed)
	}
	if trimmed := strings.TrimSpace(stderrText); trimmed != "" {
		ctx.Warn("stderr: %s", trimmed)
	}
	return outputs, nil
}

// scriptText 优先使用 script 数据输入，未连接时使用 config.default_script
// 不做 TrimSpace，避免误删脚本里有意义的尾部空行（heredoc 等）
func scriptText(ctx engine.ExecContext) string {
	if value, ok := ctx.Input("script"); ok {
		if s, ok := value.(string); ok && s != "" {
			return s
		}
	}
	return ctx.ConfigString("default_script")
}

// resolveInterpreter 把 config 字符串归一为支持的解释器，未知值 fallback bash
func resolveInterpreter(s string) string {
	switch strings.TrimSpace(s) {
	case "sh":
		return "sh"
	default:
		return "bash"
	}
}

// inputClient 读取必填的 SSH 连接句柄
func inputClient(ctx engine.ExecContext) (*clients.LinuxSshClient, error) {
	value, ok := ctx.Input("client")
	if !ok || value == nil {
		return nil, fmt.Errorf("linux_exec_script 节点缺少 client 输入")
	}
	c, ok := value.(*clients.LinuxSshClient)
	if !ok {
		return nil, fmt.Errorf("client 输入类型不是 LinuxSshConnection")
	}
	return c, nil
}

// runWithTimeout 复用 linux_exec_command 的执行模式：节点超时 + 工作流取消
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
		return fmt.Errorf("脚本执行超时: %s", timeout)
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
