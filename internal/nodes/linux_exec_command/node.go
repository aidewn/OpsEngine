// linux_exec_command 节点：通过 LinuxSshConnection 执行远程 Linux 命令

package linux_exec_command

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

const defaultTimeoutSeconds = 60

func init() { engine.Register(&Node{}) }

// Node linux_exec_command 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	minTimeout, maxTimeout := int64(1), int64(3600)
	return core.NodeTypeDef{
		TypeID:      "linux_exec_command",
		DisplayName: "Linux 执行命令",
		Category:    "remote",
		NodeKind:    core.NodeKindAction,
		Icon:        "⌨️",
		Description: "通过 Linux SSH 连接执行远程命令",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "client", Label: "SSH", PortType: core.PortTypeLinuxSsh, Required: true},
			{ID: "command", Label: "命令", PortType: core.PortTypeString},
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
			{Type: "textarea", ID: "command", Label: "命令", Required: true,
				Placeholder: "uname -a"},
			{Type: "number", ID: "timeout_seconds", Label: "超时（秒）",
				Min: &minTimeout, Max: &maxTimeout, Default: int64(defaultTimeoutSeconds)},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 创建 SSH session 并执行命令
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	sshClient, err := inputClient(ctx)
	if err != nil {
		return nil, err
	}
	command := commandText(ctx)
	if command == "" {
		return nil, fmt.Errorf("linux_exec_command 节点的 command 未配置")
	}
	timeoutSeconds := ctx.ConfigInt("timeout_seconds")
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultTimeoutSeconds
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

	ctx.Info("执行远程命令: %s", command)
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
		ctx.Error("命令执行失败，exit_code=%d", exitCode)
		if stderrText != "" {
			ctx.Error("stderr: %s", strings.TrimSpace(stderrText))
		}
		if stdoutText != "" {
			ctx.Info("stdout: %s", strings.TrimSpace(stdoutText))
		}
		return outputs, fmt.Errorf("命令执行失败，exit_code=%d: %w", exitCode, runErr)
	}

	ctx.Info("命令执行成功")
	if stdoutText != "" {
		ctx.Info("stdout: %s", strings.TrimSpace(stdoutText))
	}
	if stderrText != "" {
		ctx.Warn("stderr: %s", strings.TrimSpace(stderrText))
	}
	return outputs, nil
}

// inputClient 从 client 输入端口读取 Linux SSH 连接句柄
func inputClient(ctx engine.ExecContext) (*clients.LinuxSshClient, error) {
	value, ok := ctx.Input("client")
	if !ok || value == nil {
		return nil, fmt.Errorf("linux_exec_command 节点缺少 client 输入")
	}
	client, ok := value.(*clients.LinuxSshClient)
	if !ok {
		return nil, fmt.Errorf("client 输入类型不是 LinuxSshConnection")
	}
	return client, nil
}

// commandText 优先使用 command 数据输入，未连接时使用 config.command
func commandText(ctx engine.ExecContext) string {
	if value, ok := ctx.Input("command"); ok {
		if s, ok := value.(string); ok {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				return trimmed
			}
		}
	}
	return strings.TrimSpace(ctx.ConfigString("command"))
}

// runWithTimeout 执行命令并响应工作流取消 / 节点超时
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
		return fmt.Errorf("命令执行超时: %s", timeout)
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
