// docker_rm 节点：删除容器
// 删除后句柄已失效，因此不再输出 DockerContainerHandle，只输出已删除的 ID

package docker_rm

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
	return core.NodeTypeDef{
		TypeID:      "docker_rm",
		DisplayName: "Docker 删除容器",
		Category:    "docker",
		NodeKind:    core.NodeKindAction,
		Icon:        "🗑",
		Description: "删除容器；force=true 时连同 running 容器一起强删",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "container", Label: "Container", PortType: core.PortTypeDockerContainer, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "removed_id", Label: "ID", PortType: core.PortTypeString},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "toggle", ID: "force", Label: "强制删除（含 running）", Default: false},
			{Type: "toggle", ID: "remove_volumes", Label: "同时删除匿名 volume", Default: false},
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
		return nil, fmt.Errorf("docker_rm: 容器句柄没有可用的 Docker 客户端")
	}

	force := ctx.ConfigBool("force")
	removeVolumes := ctx.ConfigBool("remove_volumes")

	ctx.Info("删除容器 %s (force=%v, volumes=%v)", short(handle.ContainerID), force, removeVolumes)
	err = api.ContainerRemove(ctx.Context(), handle.ContainerID, container.RemoveOptions{
		Force:         force,
		RemoveVolumes: removeVolumes,
	})
	if err != nil {
		return nil, fmt.Errorf("docker_rm: ContainerRemove %s 失败: %w", short(handle.ContainerID), err)
	}

	ctx.Info("容器已删除: %s", handle.Name)
	return engine.Outputs{
		"removed_id": handle.ContainerID,
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
