// docker_run 节点：基于已 pull 的镜像创建并启动容器
// 始终 detach，节点立即返回 DockerContainerHandle，方便下游 exec/logs/stop

package docker_run

import (
	"fmt"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	"github.com/docker/docker/api/types/container"
)

func init() { engine.Register(&Node{}) }

type Node struct{}

func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "docker_run",
		DisplayName: "Docker 启动容器",
		Category:    "docker",
		NodeKind:    core.NodeKindAction,
		Icon:        "▶",
		Description: "创建并启动容器（detach），输出容器句柄供下游节点引用",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "client", Label: "Docker", PortType: core.PortTypeDockerContext, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "container", Label: "Container", PortType: core.PortTypeDockerContainer},
			{ID: "container_id", Label: "ID", PortType: core.PortTypeString},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "image", Label: "镜像", Placeholder: "nginx:1.27", Required: true},
			{Type: "text", ID: "name", Label: "容器名（可选）", Placeholder: "留空 Docker 自动生成"},
			{Type: "text", ID: "command", Label: "启动命令（可选）",
				Placeholder: `nginx -g "daemon off;"`},
			{Type: "textarea", ID: "env", Label: "环境变量（每行一个 KEY=VALUE）"},
			{Type: "textarea", ID: "ports", Label: "端口映射（每行 HOST:CONTAINER 或 HOST:CONTAINER/proto）"},
			{Type: "textarea", ID: "labels", Label: "标签（每行 key=value）"},
			{Type: "toggle", ID: "auto_remove", Label: "退出后自动删除（--rm）", Default: false},
			{Type: "select", ID: "restart_policy", Label: "重启策略",
				Options: []string{"no", "always", "on-failure", "unless-stopped"},
				Default: "no"},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	client, err := dockerClientFromInput(ctx)
	if err != nil {
		return nil, err
	}
	image := strings.TrimSpace(ctx.ConfigString("image"))
	if image == "" {
		return nil, fmt.Errorf("docker_run: image 未配置")
	}
	name := strings.TrimSpace(ctx.ConfigString("name"))

	env, err := parseEnv(ctx.ConfigString("env"))
	if err != nil {
		return nil, fmt.Errorf("docker_run: %w", err)
	}
	labels, err := parseLabels(ctx.ConfigString("labels"))
	if err != nil {
		return nil, fmt.Errorf("docker_run: %w", err)
	}
	exposedPorts, portBindings, err := parsePorts(ctx.ConfigString("ports"))
	if err != nil {
		return nil, fmt.Errorf("docker_run: %w", err)
	}

	cfg := &container.Config{
		Image:        image,
		Env:          env,
		Labels:       labels,
		ExposedPorts: exposedPorts,
	}
	// 用 sh -c 包一层，让用户能写带引号的复合命令；为空则用镜像 ENTRYPOINT
	if cmd := strings.TrimSpace(ctx.ConfigString("command")); cmd != "" {
		cfg.Cmd = []string{"sh", "-c", cmd}
	}

	hostCfg := &container.HostConfig{
		AutoRemove:    ctx.ConfigBool("auto_remove"),
		PortBindings:  portBindings,
		RestartPolicy: restartPolicy(ctx.ConfigString("restart_policy")),
	}

	api := client.API()
	ctx.Info("创建容器 image=%s name=%s ...", image, name)
	createResp, err := api.ContainerCreate(ctx.Context(), cfg, hostCfg, nil, nil, name)
	if err != nil {
		return nil, fmt.Errorf("docker_run: ContainerCreate 失败: %w", err)
	}
	for _, w := range createResp.Warnings {
		ctx.Warn("create warning: %s", w)
	}

	ctx.Info("启动容器 %s ...", createResp.ID[:12])
	if err := api.ContainerStart(ctx.Context(), createResp.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("docker_run: ContainerStart %s 失败: %w", createResp.ID[:12], err)
	}

	// Inspect 拿到规范化的 Name / Image，作为句柄的展示字段
	insp, err := api.ContainerInspect(ctx.Context(), createResp.ID)
	resolvedName := name
	resolvedImage := image
	if err == nil {
		resolvedName = insp.Name
		if insp.Config != nil && insp.Config.Image != "" {
			resolvedImage = insp.Config.Image
		}
	}

	handle, err := clients.NewDockerContainerHandle(client, createResp.ID, resolvedName, resolvedImage)
	if err != nil {
		return nil, fmt.Errorf("docker_run: 构造容器句柄失败: %w", err)
	}

	ctx.Info("容器已启动: id=%s name=%s", createResp.ID[:12], handle.Name)
	return engine.Outputs{
		"container":    handle,
		"container_id": createResp.ID,
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
