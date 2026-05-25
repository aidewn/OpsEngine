// docker_exec 节点：在已运行容器中执行命令
// 消费 DockerContainerHandle；返回 stdout/stderr/exit_code

package docker_exec

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

func init() { engine.Register(&Node{}) }

type Node struct{}

func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "docker_exec",
		DisplayName: "Docker 执行命令",
		Category:    "docker",
		NodeKind:    core.NodeKindAction,
		Icon:        "▶",
		Description: "在运行中的容器内执行命令，返回 stdout / stderr / exit_code",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "container", Label: "Container", PortType: core.PortTypeDockerContainer, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "stdout", Label: "stdout", PortType: core.PortTypeString},
			{ID: "stderr", Label: "stderr", PortType: core.PortTypeString},
			{ID: "exit_code", Label: "exit", PortType: core.PortTypeInt},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "command", Label: "命令（sh -c 包裹）", Placeholder: "nginx -t", Required: true},
			{Type: "text", ID: "workdir", Label: "工作目录（可选）"},
			{Type: "text", ID: "user", Label: "执行用户（可选）"},
			{Type: "toggle", ID: "tty", Label: "分配 TTY", Default: false},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	handle, err := containerHandleFromInput(ctx)
	if err != nil {
		return nil, err
	}
	cmd := strings.TrimSpace(ctx.ConfigString("command"))
	if cmd == "" {
		return nil, fmt.Errorf("docker_exec: command 未配置")
	}
	tty := ctx.ConfigBool("tty")

	api := handle.API()
	if api == nil {
		return nil, fmt.Errorf("docker_exec: 容器句柄没有可用的 Docker 客户端")
	}

	execCfg := container.ExecOptions{
		Cmd:          []string{"sh", "-c", cmd},
		WorkingDir:   strings.TrimSpace(ctx.ConfigString("workdir")),
		User:         strings.TrimSpace(ctx.ConfigString("user")),
		Tty:          tty,
		AttachStdout: true,
		AttachStderr: true,
	}

	ctx.Info("exec in %s: %s", short(handle.ContainerID), cmd)
	created, err := api.ContainerExecCreate(ctx.Context(), handle.ContainerID, execCfg)
	if err != nil {
		return nil, fmt.Errorf("docker_exec: ContainerExecCreate 失败: %w", err)
	}

	resp, err := api.ContainerExecAttach(ctx.Context(), created.ID, container.ExecAttachOptions{Tty: tty})
	if err != nil {
		return nil, fmt.Errorf("docker_exec: ContainerExecAttach 失败: %w", err)
	}
	defer resp.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	if tty {
		// TTY 模式下 stdout/stderr 不分流，全部走 stdout
		if _, err := io.Copy(&stdoutBuf, resp.Reader); err != nil && err != io.EOF {
			return nil, fmt.Errorf("docker_exec: 读取 TTY 输出失败: %w", err)
		}
	} else {
		if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, resp.Reader); err != nil && err != io.EOF {
			return nil, fmt.Errorf("docker_exec: 读取 exec 输出失败: %w", err)
		}
	}

	insp, err := api.ContainerExecInspect(ctx.Context(), created.ID)
	if err != nil {
		return nil, fmt.Errorf("docker_exec: ContainerExecInspect 失败: %w", err)
	}

	ctx.Info("exec 完成 exit=%d stdout=%dB stderr=%dB",
		insp.ExitCode, stdoutBuf.Len(), stderrBuf.Len())

	return engine.Outputs{
		"stdout":    stdoutBuf.String(),
		"stderr":    stderrBuf.String(),
		"exit_code": int64(insp.ExitCode),
	}, nil
}

func containerHandleFromInput(ctx engine.ExecContext) (*clients.DockerContainerHandle, error) {
	v, ok := ctx.Input("container")
	if !ok || v == nil {
		return nil, fmt.Errorf("container 输入端口未连接 DockerContainerHandle")
	}
	h, ok := v.(*clients.DockerContainerHandle)
	if !ok || h == nil {
		return nil, fmt.Errorf("container 输入端口类型不是 *DockerContainerHandle，得到 %T", v)
	}
	return h, nil
}

func short(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
