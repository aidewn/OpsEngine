// docker_filter 节点：按 name/label/status 找到一个已存在的容器
// 输出 DockerContainerHandle 供 docker_exec / docker_logs 等节点消费
// 这是"选择器节点"模式（参照 linux_open_file → LinuxFileHandle）

package docker_filter

import (
	"fmt"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

func init() { engine.Register(&Node{}) }

type Node struct{}

func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "docker_filter",
		DisplayName: "Docker 查找容器",
		Category:    "docker",
		NodeKind:    core.NodeKindAction,
		Icon:        "🔍",
		Description: "按 name / label / status 找到容器并输出句柄，供下游 exec / logs 引用",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "client", Label: "Docker", PortType: core.PortTypeDockerContext, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "container", Label: "Container", PortType: core.PortTypeDockerContainer},
			{ID: "container_id", Label: "ID", PortType: core.PortTypeString},
			{ID: "found", Label: "Found", PortType: core.PortTypeBool},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "name", Label: "容器名（可选，子串匹配）", Placeholder: "nginx-demo"},
			{Type: "text", ID: "label", Label: "标签过滤（可选，key=value）"},
			{Type: "select", ID: "status", Label: "状态",
				Options: []string{"running", "any", "exited"},
				Default: "running"},
			{Type: "toggle", ID: "require_unique", Label: "多匹配时报错（require_unique）", Default: false},
			{Type: "toggle", ID: "error_if_missing", Label: "未找到时报错", Default: true},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	client, err := dockerClientFromInput(ctx)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(ctx.ConfigString("name"))
	label := strings.TrimSpace(ctx.ConfigString("label"))
	status := strings.TrimSpace(ctx.ConfigString("status"))
	if status == "" {
		status = "running"
	}
	requireUnique := ctx.ConfigBool("require_unique")
	errIfMissing := ctx.ConfigBool("error_if_missing")

	args := filters.NewArgs()
	if name != "" {
		args.Add("name", name)
	}
	if label != "" {
		args.Add("label", label)
	}
	switch status {
	case "running":
		args.Add("status", "running")
	case "exited":
		args.Add("status", "exited")
	}

	// status=any 需要 All=true 才能扫到非 running 容器
	all := status != "running"

	summaries, err := client.API().ContainerList(ctx.Context(), container.ListOptions{
		All:     all,
		Filters: args,
	})
	if err != nil {
		return nil, fmt.Errorf("docker_filter: ContainerList 失败: %w", err)
	}

	if len(summaries) == 0 {
		if errIfMissing {
			return nil, fmt.Errorf("docker_filter: 未找到匹配容器 (name=%q label=%q status=%s)", name, label, status)
		}
		ctx.Warn("未找到匹配容器，found=false")
		return engine.Outputs{
			"container":    (*clients.DockerContainerHandle)(nil),
			"container_id": "",
			"found":        false,
		}, nil
	}

	if requireUnique && len(summaries) > 1 {
		return nil, fmt.Errorf("docker_filter: 匹配到 %d 个容器，require_unique 模式下应当唯一", len(summaries))
	}

	// 优先取 running，回退首个
	picked := summaries[0]
	for _, s := range summaries {
		if s.State == "running" {
			picked = s
			break
		}
	}

	pickedName := ""
	if len(picked.Names) > 0 {
		pickedName = picked.Names[0]
	}

	handle, err := clients.NewDockerContainerHandle(client, picked.ID, pickedName, picked.Image)
	if err != nil {
		return nil, fmt.Errorf("docker_filter: 构造容器句柄失败: %w", err)
	}

	ctx.Info("命中容器: id=%s name=%s state=%s (共 %d 个匹配)",
		picked.ID[:12], handle.Name, picked.State, len(summaries))

	return engine.Outputs{
		"container":    handle,
		"container_id": picked.ID,
		"found":        true,
	}, nil
}

func dockerClientFromInput(ctx engine.ExecContext) (*clients.DockerClient, error) {
	v, ok := ctx.Input("client")
	if !ok || v == nil {
		return nil, fmt.Errorf("client 输入端口未连接 DockerContext")
	}
	c, ok := v.(*clients.DockerClient)
	if !ok {
		return nil, fmt.Errorf("client 输入端口类型不是 *DockerClient，得到 %T", v)
	}
	return c, nil
}
