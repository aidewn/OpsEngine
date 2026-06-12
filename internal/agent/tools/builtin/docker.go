// Docker 只读工具：列容器 / 看日志 / 看详情 / 列镜像。
//
// 列容器复用 probe.Run（env_probe_docker_containers 已经实现了 dial + ContainerList + 关闭）。
// 其它需要 DockerClient 直接 API 的（日志、详情、镜像列表）走本地 dialDockerForTool 拨号。

package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"OpsEngine/internal/agent/tools"
	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/probe"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
)

const (
	dockerDefaultSocket = "/var/run/docker.sock"
	dockerToolTimeout   = 15 * time.Second
)

// dialDockerForTool 按 Docker 配置（local / over_ssh）拨号，返回可关闭的 DockerClient。
// 复用了 env_connect_docker / env_probe_docker_containers 的等价逻辑，但内联在工具层，
// 避免对 nodes/probe 包反向依赖。
func dialDockerForTool(env core.EnvironmentDef, cfg core.EnvConfigItem) (*clients.DockerClient, error) {
	mode := stringFieldDocker(cfg.Fields, "mode")
	if mode == "" {
		mode = "over_ssh"
	}
	socketPath := stringFieldDocker(cfg.Fields, "socket_path")
	switch mode {
	case "local":
		// 空 socket_path 由 NewDockerClientLocal 自动探测（Windows npipe / Linux unix socket）
		return clients.NewDockerClientLocal(socketPath)
	case "ssh_cli", "over_ssh":
		if socketPath == "" {
			socketPath = dockerDefaultSocket
		}
		sshConfigID := stringFieldDocker(cfg.Fields, "ssh_config_id")
		if sshConfigID == "" {
			return nil, fmt.Errorf("Docker 配置缺少 ssh_config_id")
		}
		var sshCfg *core.EnvConfigItem
		for i := range env.Configs {
			if env.Configs[i].ID == sshConfigID {
				if env.Configs[i].Kind != core.EnvConfigKindSSH {
					return nil, fmt.Errorf("ssh_config_id %s 类型 %s 不是 ssh", sshConfigID, env.Configs[i].Kind)
				}
				sshCfg = &env.Configs[i]
				break
			}
		}
		if sshCfg == nil {
			return nil, fmt.Errorf("ssh_config_id 未找到: %s", sshConfigID)
		}
		host := stringFieldDocker(sshCfg.Fields, "host")
		user := stringFieldDocker(sshCfg.Fields, "user")
		password := stringFieldDocker(sshCfg.Fields, "password")
		port := intFieldDocker(sshCfg.Fields, "port", 22)
		timeout := intFieldDocker(sshCfg.Fields, "timeout_seconds", 10)
		if host == "" || user == "" {
			return nil, fmt.Errorf("引用的 SSH 配置缺少 host/user")
		}
		linuxClient, err := clients.DialLinuxSsh(host, port, user, password, timeout)
		if err != nil {
			return nil, err
		}
		var dockerClient *clients.DockerClient
		if mode == "ssh_cli" {
			dockerClient, err = clients.NewDockerClientOverSSHCLI(linuxClient.Client(), host, port, user)
		} else {
			dockerClient, err = clients.NewDockerClientOverSSH(linuxClient.Client(), host, port, user, socketPath)
		}
		if err != nil {
			_ = linuxClient.Close()
			return nil, fmt.Errorf("构造 Docker 客户端失败: %w", err)
		}
		// 保留 host:port 给后续描述使用
		_ = net.JoinHostPort(host, strconv.Itoa(port))
		return dockerClient, nil
	}
	return nil, fmt.Errorf("不支持的 Docker mode=%s（仅支持 local / over_ssh）", mode)
}

// stringFieldDocker / intFieldDocker：Docker 配置字段读取的本地实现（避免依赖 nodes/ 包私有函数）。
func stringFieldDocker(fields map[string]any, key string) string {
	if fields == nil {
		return ""
	}
	if v, ok := fields[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func intFieldDocker(fields map[string]any, key string, fallback int) int {
	if fields == nil {
		return fallback
	}
	v, ok := fields[key]
	if !ok {
		return fallback
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return fallback
}

// ── docker_list_containers ─────────────────────────────────────

// DockerListContainers 列出 Docker daemon 上的容器；复用 probe.Run 避免重写逻辑。
type DockerListContainers struct{}

func (DockerListContainers) Spec() tools.Spec {
	return tools.Spec{
		Name: "docker_list_containers",
		Description: "列出当前环境内 Docker 主机上的容器（默认只看运行中）。" +
			"返回精简 JSON：id/name/image/state/status。用于了解服务部署情况、找异常容器。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"all":         {Type: "boolean", Description: "是否包含已停止容器，默认 false。", Default: false},
			"filter_name": {Type: "string", Description: "按名字子串过滤（可选）。"},
		},
	}
}

func (DockerListContainers) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	cfg, env, err := pickEnvConfig(ctx, core.EnvConfigKindDocker)
	if err != nil {
		return tools.Result{}, err
	}
	res, err := probe.Run("env_probe_docker_containers", env, cfg.ID, map[string]any{
		"all":         argBool(args, "all", false),
		"filter_name": argStringOptional(args, "filter_name"),
	})
	if err != nil {
		return tools.Result{}, err
	}
	// 把 ProbeResult 转成可读的 JSON 行
	type entry struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Image  string `json:"image"`
		State  string `json:"state"`
		Status string `json:"status"`
	}
	entries := make([]entry, 0, len(res.Items))
	for _, it := range res.Items {
		image, _ := it.Meta["image"].(string)
		state, _ := it.Meta["state"].(string)
		status, _ := it.Meta["status"].(string)
		entries = append(entries, entry{ID: it.Key, Name: it.Label, Image: image, State: state, Status: status})
	}
	data, _ := json.MarshalIndent(entries, "", "  ")
	return tools.Result{
		Output:         tools.TruncateOutput(string(data)),
		DisplaySummary: fmt.Sprintf("docker_list_containers @ %s（%d 个）", env.Name, len(entries)),
		View:           listView("container_list", fmt.Sprintf("容器 · %s（%d）", env.Name, len(entries)), string(data)),
	}, nil
}

// ── docker_container_logs ──────────────────────────────────────

// DockerContainerLogs 取容器日志尾部。
type DockerContainerLogs struct{}

func (DockerContainerLogs) Spec() tools.Spec {
	return tools.Spec{
		Name: "docker_container_logs",
		Description: "读取指定 Docker 容器的日志尾部。container 可以是名字或 id（短前缀也行）。" +
			"用于排查应用错误、查看最近的运行输出。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"container": {Type: "string", Description: "容器名或 ID。", Required: true},
			"tail":      {Type: "integer", Description: "读取尾部多少行，默认 200，上限 1000。", Default: 200},
		},
	}
}

func (DockerContainerLogs) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	target, err := argString(args, "container")
	if err != nil {
		return tools.Result{}, err
	}
	tail := argInt(args, "tail", 200)
	if tail < 1 {
		tail = 200
	}
	if tail > 1000 {
		tail = 1000
	}
	cfg, env, err := pickEnvConfig(ctx, core.EnvConfigKindDocker)
	if err != nil {
		return tools.Result{}, err
	}
	dockerClient, err := dialDockerForTool(env, cfg)
	if err != nil {
		return tools.Result{}, err
	}
	defer dockerClient.Close()

	apiCtx, cancel := context.WithTimeout(context.Background(), dockerToolTimeout)
	defer cancel()

	reader, err := dockerClient.API().ContainerLogs(apiCtx, target, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", tail),
		Timestamps: false,
	})
	if err != nil {
		return tools.Result{}, fmt.Errorf("ContainerLogs 失败: %w", err)
	}
	defer reader.Close()
	// docker API 返回的 logs 是 multiplexed stream（stdout/stderr 共享同一流）；
	// 直接 io.ReadAll 后截断到 MaxOutputBytes，损失流头不影响人类阅读。
	raw, err := io.ReadAll(reader)
	if err != nil {
		return tools.Result{}, fmt.Errorf("读取日志失败: %w", err)
	}
	return tools.Result{
		Output:         tools.TruncateOutput(string(raw)),
		DisplaySummary: fmt.Sprintf("docker_container_logs %s (%d 行)", target, tail),
	}, nil
}

// ── docker_container_inspect ───────────────────────────────────

// DockerContainerInspect 查看容器详情。
type DockerContainerInspect struct{}

func (DockerContainerInspect) Spec() tools.Spec {
	return tools.Spec{
		Name: "docker_container_inspect",
		Description: "查看指定 Docker 容器的详细信息（镜像、命令、网络、端口、环境变量、挂载、状态）。" +
			"用于了解容器具体配置；container 可以是名字或 ID。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"container": {Type: "string", Description: "容器名或 ID。", Required: true},
		},
	}
}

func (DockerContainerInspect) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	target, err := argString(args, "container")
	if err != nil {
		return tools.Result{}, err
	}
	cfg, env, err := pickEnvConfig(ctx, core.EnvConfigKindDocker)
	if err != nil {
		return tools.Result{}, err
	}
	dockerClient, err := dialDockerForTool(env, cfg)
	if err != nil {
		return tools.Result{}, err
	}
	defer dockerClient.Close()

	apiCtx, cancel := context.WithTimeout(context.Background(), dockerToolTimeout)
	defer cancel()

	insp, err := dockerClient.API().ContainerInspect(apiCtx, target)
	if err != nil {
		return tools.Result{}, fmt.Errorf("ContainerInspect 失败: %w", err)
	}
	// 精简结构：只暴露常用字段，避免把整段 NetworkSettings/Mounts 灌满 context
	type summary struct {
		ID       string            `json:"id"`
		Name     string            `json:"name"`
		Image    string            `json:"image"`
		State    string            `json:"state"`
		Status   string            `json:"status"`
		Started  string            `json:"started_at"`
		Cmd      []string          `json:"cmd,omitempty"`
		Env      []string          `json:"env,omitempty"`
		Ports    map[string]string `json:"ports,omitempty"`
		Networks []string          `json:"networks,omitempty"`
		Mounts   []string          `json:"mounts,omitempty"`
	}
	ports := map[string]string{}
	for p, bindings := range insp.NetworkSettings.Ports {
		if len(bindings) > 0 {
			ports[string(p)] = bindings[0].HostIP + ":" + bindings[0].HostPort
		} else {
			ports[string(p)] = ""
		}
	}
	networks := make([]string, 0, len(insp.NetworkSettings.Networks))
	for n := range insp.NetworkSettings.Networks {
		networks = append(networks, n)
	}
	mounts := make([]string, 0, len(insp.Mounts))
	for _, m := range insp.Mounts {
		mounts = append(mounts, fmt.Sprintf("%s → %s (%s)", m.Source, m.Destination, m.Mode))
	}
	s := summary{
		ID:       insp.ID,
		Name:     insp.Name,
		Image:    insp.Config.Image,
		State:    insp.State.Status,
		Status:   insp.State.Status,
		Started:  insp.State.StartedAt,
		Cmd:      insp.Config.Cmd,
		Env:      insp.Config.Env,
		Ports:    ports,
		Networks: networks,
		Mounts:   mounts,
	}
	data, _ := json.MarshalIndent(s, "", "  ")
	return tools.Result{
		Output:         tools.TruncateOutput(string(data)),
		DisplaySummary: fmt.Sprintf("docker_container_inspect %s", target),
	}, nil
}

// ── docker_list_images ─────────────────────────────────────────

// DockerListImages 列镜像。
type DockerListImages struct{}

func (DockerListImages) Spec() tools.Spec {
	return tools.Spec{
		Name:        "docker_list_images",
		Description: "列出 Docker 主机上的本地镜像（含 tag、ID、大小、创建时间）。用于了解服务可用的镜像版本。",
		Tier:        tools.TierRead,
		Params:      map[string]tools.ParamSpec{},
	}
}

func (DockerListImages) Execute(ctx tools.ToolContext, _ map[string]any) (tools.Result, error) {
	cfg, env, err := pickEnvConfig(ctx, core.EnvConfigKindDocker)
	if err != nil {
		return tools.Result{}, err
	}
	dockerClient, err := dialDockerForTool(env, cfg)
	if err != nil {
		return tools.Result{}, err
	}
	defer dockerClient.Close()

	apiCtx, cancel := context.WithTimeout(context.Background(), dockerToolTimeout)
	defer cancel()

	images, err := dockerClient.API().ImageList(apiCtx, image.ListOptions{})
	if err != nil {
		return tools.Result{}, fmt.Errorf("ImageList 失败: %w", err)
	}
	type entry struct {
		ID      string   `json:"id"`
		Tags    []string `json:"tags,omitempty"`
		SizeMB  int64    `json:"size_mb"`
		Created int64    `json:"created"`
	}
	entries := make([]entry, 0, len(images))
	for _, img := range images {
		entries = append(entries, entry{
			ID:      img.ID,
			Tags:    img.RepoTags,
			SizeMB:  img.Size / (1024 * 1024),
			Created: img.Created,
		})
	}
	data, _ := json.MarshalIndent(entries, "", "  ")
	return tools.Result{
		Output:         tools.TruncateOutput(string(data)),
		DisplaySummary: fmt.Sprintf("docker_list_images @ %s（%d 个）", env.Name, len(entries)),
	}, nil
}
