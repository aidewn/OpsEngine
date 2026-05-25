// docker_stop 节点：优雅停止容器（SIGTERM + 超时后 SIGKILL）
// 句柄在停止后仍然有效（容器还在，只是没在 running），可继续接 docker_rm / docker_restart

package docker_stop

import (
	"fmt"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	"github.com/docker/docker/api/types/container"
)

func init() { engine.Register(&Node{}) }

type Node struct{}

func (Node) TypeDef() core.NodeTypeDef {
	minTimeout, maxTimeout := int64(0), int64(3600)
	return core.NodeTypeDef{
		TypeID:      "docker_stop",
		DisplayName: "Docker 停止容器",
		Category:    "docker",
		NodeKind:    core.NodeKindAction,
		Icon:        "⏹",
		Description: "停止运行中的容器（默认 SIGTERM，超时后 SIGKILL）",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "container", Label: "Container", PortType: core.PortTypeDockerContainer, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "container", Label: "Container", PortType: core.PortTypeDockerContainer},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "number", ID: "timeout_seconds", Label: "优雅停止超时（秒）",
				Min: &minTimeout, Max: &maxTimeout, Default: int64(10)},
			{Type: "text", ID: "signal", Label: "信号（可选）", Placeholder: "SIGTERM"},
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
		return nil, fmt.Errorf("docker_stop: 容器句柄没有可用的 Docker 客户端")
	}

	opts := container.StopOptions{}
	if t := int(ctx.ConfigInt("timeout_seconds")); t > 0 {
		opts.Timeout = &t
	}
	if sig := ctx.ConfigString("signal"); sig != "" {
		opts.Signal = sig
	}

	ctx.Info("停止容器 %s (timeout=%ds)", short(handle.ContainerID), ctx.ConfigInt("timeout_seconds"))
	if err := api.ContainerStop(ctx.Context(), handle.ContainerID, opts); err != nil {
		return nil, fmt.Errorf("docker_stop: ContainerStop %s 失败: %w", short(handle.ContainerID), err)
	}

	ctx.Info("容器已停止: %s", handle.Name)
	// 透传同一个句柄，方便下游 docker_rm 直接接
	return engine.Outputs{
		"container": handle,
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
