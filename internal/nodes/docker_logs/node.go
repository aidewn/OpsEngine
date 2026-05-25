// docker_logs 节点：抓取容器日志（一次性，非 follow）
// 消费 DockerContainerHandle；返回合并后的 stdout+stderr 文本

package docker_logs

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
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
	minTail, maxTail := int64(0), int64(100000)
	return core.NodeTypeDef{
		TypeID:      "docker_logs",
		DisplayName: "Docker 抓取日志",
		Category:    "docker",
		NodeKind:    core.NodeKindAction,
		Icon:        "📜",
		Description: "抓取容器日志（一次性，非 follow），返回合并后的文本",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "container", Label: "Container", PortType: core.PortTypeDockerContainer, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "logs", Label: "logs", PortType: core.PortTypeString},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "number", ID: "tail", Label: "tail 行数（0 = 全部）",
				Min: &minTail, Max: &maxTail, Default: int64(100)},
			{Type: "text", ID: "since", Label: "since（可选，如 10m / RFC3339）"},
			{Type: "toggle", ID: "timestamps", Label: "包含时间戳", Default: false},
			{Type: "toggle", ID: "include_stderr", Label: "包含 stderr", Default: true},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	handle, err := containerHandleFromInput(ctx)
	if err != nil {
		return nil, err
	}
	api := handle.API()
	if api == nil {
		return nil, fmt.Errorf("docker_logs: 容器句柄没有可用的 Docker 客户端")
	}

	tail := ctx.ConfigInt("tail")
	tailStr := "all"
	if tail > 0 {
		tailStr = strconv.FormatInt(tail, 10)
	}
	includeStderr := ctx.ConfigBool("include_stderr")

	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: includeStderr,
		Tail:       tailStr,
		Since:      strings.TrimSpace(ctx.ConfigString("since")),
		Timestamps: ctx.ConfigBool("timestamps"),
		Follow:     false,
	}

	ctx.Info("抓取容器日志 %s (tail=%s)", short(handle.ContainerID), tailStr)
	rc, err := api.ContainerLogs(ctx.Context(), handle.ContainerID, opts)
	if err != nil {
		return nil, fmt.Errorf("docker_logs: ContainerLogs 失败: %w", err)
	}
	defer rc.Close()

	var buf bytes.Buffer
	// stderr 既然要合并显示，就让 stdcopy 把两条流写到同一个 buf
	stderrSink := io.Writer(io.Discard)
	if includeStderr {
		stderrSink = &buf
	}
	if _, err := stdcopy.StdCopy(&buf, stderrSink, rc); err != nil && err != io.EOF {
		return nil, fmt.Errorf("docker_logs: 读取日志流失败: %w", err)
	}

	ctx.Info("日志读取完成 %dB", buf.Len())
	return engine.Outputs{
		"logs": buf.String(),
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
