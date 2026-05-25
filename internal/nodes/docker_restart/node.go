// docker_restart 节点：重启容器
// 句柄保持有效，可继续下接 exec / logs

package docker_restart

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
		TypeID:      "docker_restart",
		DisplayName: "Docker 重启容器",
		Category:    "docker",
		NodeKind:    core.NodeKindAction,
		Icon:        "🔄",
		Description: "重启容器（先 stop 再 start，单次原子操作）",
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
		return nil, fmt.Errorf("docker_restart: 容器句柄没有可用的 Docker 客户端")
	}

	opts := container.StopOptions{}
	if t := int(ctx.ConfigInt("timeout_seconds")); t > 0 {
		opts.Timeout = &t
	}
	if sig := ctx.ConfigString("signal"); sig != "" {
		opts.Signal = sig
	}

	ctx.Info("重启容器 %s ...", short(handle.ContainerID))
	if err := api.ContainerRestart(ctx.Context(), handle.ContainerID, opts); err != nil {
		return nil, fmt.Errorf("docker_restart: ContainerRestart %s 失败: %w", short(handle.ContainerID), err)
	}

	ctx.Info("容器已重启: %s", handle.Name)
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
