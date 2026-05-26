// env_probe_docker_containers 节点
// Pure 节点（无 exec 端口）；按 resolve_mode 工作：
//   static  —— 不连远程，从 probe_snapshot 还原 output 端口值
//   dynamic —— 调用本包注册的 Probe 函数实时列容器
// 编辑态「探测一次」复用 Probe 函数

package env_probe_docker_containers

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	"OpsEngine/internal/probe"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

// TypeID 节点与探测函数共用同一 ID
const TypeID = "env_probe_docker_containers"

const defaultSocketPath = "/var/run/docker.sock"

func init() {
	engine.Register(&Node{})
	probe.Register(TypeID, Probe)
}

// Node Docker 容器探测节点
type Node struct{}

// TypeDef 节点元信息
// 输出端口对应 plan §6.2：selected_id / selected_name / names
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      TypeID,
		DisplayName: "环境 · Docker 列容器",
		Category:    "environment",
		NodeKind:    core.NodeKindPure,
		Icon:        "🐳",
		Description: "列出环境 Docker 守护进程上的容器；编辑态可点选，运行态按 resolve_mode 还原或重探",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "selected_id", Label: "selected_id", PortType: core.PortTypeString},
			{ID: "selected_name", Label: "selected_name", PortType: core.PortTypeString},
			{ID: "names", Label: "names", PortType: core.PortTypeAny},
			{ID: "count", Label: "count", PortType: core.PortTypeInt},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "env_select", ID: "environment_id", Label: "环境", Required: true},
			{Type: "env_config_select", ID: "config_id", Label: "Docker 配置",
				Required: true, ConfigKindFilter: string(core.EnvConfigKindDocker)},
			{Type: "toggle", ID: "all", Label: "包含已停止容器", Default: false},
			{Type: "text", ID: "filter_name", Label: "name 过滤（子串匹配，可选）"},
			{Type: "select", ID: "resolve_mode", Label: "运行模式",
				Options: []string{"static", "dynamic"}, Default: "static"},
		},
		ExecutionMode: core.ExecutionModeFlow,
	}
}

// Execute Pure 节点求值入口
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	mode := strings.TrimSpace(ctx.ConfigString("resolve_mode"))
	if mode == "" {
		mode = "static"
	}
	if mode == "dynamic" {
		return executeDynamic(ctx)
	}
	return executeStatic(ctx)
}

// executeStatic 从 probe_snapshot 还原 output
// items[i].Key = container ID；items[i].Label = container name
func executeStatic(ctx engine.ExecContext) (engine.Outputs, error) {
	snap, _ := ctx.Config("probe_snapshot").(map[string]any)
	if snap == nil {
		return engine.Outputs{
			"selected_id":   "",
			"selected_name": "",
			"names":         []string{},
			"count":         int64(0),
		}, nil
	}
	pickedID, _ := snap["picked_key"].(string)
	pickedName, _ := snap["picked_label"].(string)
	items := snapshotItems(snap["items"])
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Label)
	}
	return engine.Outputs{
		"selected_id":   pickedID,
		"selected_name": pickedName,
		"names":         names,
		"count":         int64(len(items)),
	}, nil
}

// executeDynamic 实时探测；selected_* 沿用 snapshot，否则取首项兜底
func executeDynamic(ctx engine.ExecContext) (engine.Outputs, error) {
	envID := strings.TrimSpace(ctx.ConfigString("environment_id"))
	configID := strings.TrimSpace(ctx.ConfigString("config_id"))
	if envID == "" || configID == "" {
		return nil, fmt.Errorf("environment_id / config_id 未配置")
	}
	envStore := ctx.EnvironmentStore()
	if envStore == nil {
		return nil, fmt.Errorf("引擎未配置 environmentStore")
	}
	env, err := envStore.Get(envID)
	if err != nil {
		return nil, err
	}

	nodeConfig := map[string]any{
		"all":         ctx.ConfigBool("all"),
		"filter_name": ctx.ConfigString("filter_name"),
	}
	res, err := Probe(env, configID, nodeConfig)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(res.Items))
	for _, it := range res.Items {
		names = append(names, it.Label)
	}

	pickedID := ""
	pickedName := ""
	if snap, ok := ctx.Config("probe_snapshot").(map[string]any); ok {
		pickedID, _ = snap["picked_key"].(string)
		pickedName, _ = snap["picked_label"].(string)
	}
	// snapshot 缺失或 picked 已失效时用首项兜底
	if pickedID == "" {
		if len(res.Items) > 0 {
			pickedID = res.Items[0].Key
			pickedName = res.Items[0].Label
		}
	}

	return engine.Outputs{
		"selected_id":   pickedID,
		"selected_name": pickedName,
		"names":         names,
		"count":         int64(len(res.Items)),
	}, nil
}

// snapshotItems 把 probe_snapshot.items 反序列化为 ProbeItem 列表
func snapshotItems(raw any) []probe.ProbeItem {
	arr, _ := raw.([]any)
	if len(arr) == 0 {
		return nil
	}
	out := make([]probe.ProbeItem, 0, len(arr))
	for _, v := range arr {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		item := probe.ProbeItem{}
		item.Key, _ = m["key"].(string)
		item.Label, _ = m["label"].(string)
		if meta, ok := m["meta"].(map[string]any); ok {
			item.Meta = meta
		}
		out = append(out, item)
	}
	return out
}

// Probe Docker 容器列表探测
// 自行拨号 → ContainerList → 关闭，避免持有长连接
func Probe(env core.EnvironmentDef, configID string, nodeConfig map[string]any) (probe.ProbeResult, error) {
	dockerCfg, err := findDockerConfig(env, configID)
	if err != nil {
		return probe.ProbeResult{}, err
	}
	mode := strings.TrimSpace(stringField(dockerCfg.Fields, "mode"))
	if mode == "" {
		mode = "over_ssh"
	}
	if mode != "over_ssh" {
		return probe.ProbeResult{}, fmt.Errorf("暂不支持 Docker mode=%s（Phase 3 仅 over_ssh）", mode)
	}

	sshConfigID := strings.TrimSpace(stringField(dockerCfg.Fields, "ssh_config_id"))
	if sshConfigID == "" {
		return probe.ProbeResult{}, fmt.Errorf("Docker 配置缺少 ssh_config_id")
	}
	sshCfg, err := findSSHConfig(env, sshConfigID)
	if err != nil {
		return probe.ProbeResult{}, err
	}

	socketPath := strings.TrimSpace(stringField(dockerCfg.Fields, "socket_path"))
	if socketPath == "" {
		socketPath = defaultSocketPath
	}
	host := stringField(sshCfg.Fields, "host")
	user := stringField(sshCfg.Fields, "user")
	password := stringField(sshCfg.Fields, "password")
	port := intField(sshCfg.Fields, "port", 22)
	timeout := intField(sshCfg.Fields, "timeout_seconds", 10)
	if host == "" || user == "" || password == "" {
		return probe.ProbeResult{}, fmt.Errorf("引用的 SSH 配置缺少 host/user/password")
	}

	linuxClient, err := clients.DialLinuxSsh(host, port, user, password, timeout)
	if err != nil {
		return probe.ProbeResult{}, err
	}
	// 后续 DockerClient 接管 ssh.Client；若构造失败需自己关 ssh
	dockerClient, err := clients.NewDockerClientOverSSH(linuxClient.Client(), host, port, user, socketPath)
	if err != nil {
		_ = linuxClient.Close()
		return probe.ProbeResult{}, fmt.Errorf("构造 Docker 客户端失败: %w", err)
	}
	defer dockerClient.Close()

	// 编辑态无 ctx，超时由 DockerClient HTTP transport 内部 SSH dial 控制
	pingCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := dockerClient.Ping(pingCtx); err != nil {
		return probe.ProbeResult{}, fmt.Errorf("Docker daemon 不可达: %w", err)
	}

	args := filters.NewArgs()
	if name := strings.TrimSpace(stringField(nodeConfig, "filter_name")); name != "" {
		args.Add("name", name)
	}
	summaries, err := dockerClient.API().ContainerList(pingCtx, container.ListOptions{
		All:     boolField(nodeConfig, "all", false),
		Filters: args,
	})
	if err != nil {
		return probe.ProbeResult{}, fmt.Errorf("ContainerList 失败: %w", err)
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	_ = addr // 仅用于潜在的日志，留作占位避免误删

	items := make([]probe.ProbeItem, 0, len(summaries))
	for _, s := range summaries {
		name := ""
		if len(s.Names) > 0 {
			name = strings.TrimPrefix(s.Names[0], "/")
		}
		items = append(items, probe.ProbeItem{
			Key:   s.ID,
			Label: name,
			Meta: map[string]any{
				"image":  s.Image,
				"state":  s.State,
				"status": s.Status,
			},
		})
	}
	return probe.ProbeResult{Items: items}, nil
}

// findDockerConfig 在环境中按 configID 定位 docker 配置
func findDockerConfig(env core.EnvironmentDef, configID string) (*core.EnvConfigItem, error) {
	for i := range env.Configs {
		c := &env.Configs[i]
		if c.ID == configID {
			if c.Kind != core.EnvConfigKindDocker {
				return nil, fmt.Errorf("配置 %s 类型 %s 不是 docker", configID, c.Kind)
			}
			return c, nil
		}
	}
	return nil, fmt.Errorf("配置未找到: %s", configID)
}

// findSSHConfig 在环境中按 ID 定位 ssh 配置（被 docker over_ssh 反向引用）
func findSSHConfig(env core.EnvironmentDef, configID string) (*core.EnvConfigItem, error) {
	for i := range env.Configs {
		c := &env.Configs[i]
		if c.ID == configID {
			if c.Kind != core.EnvConfigKindSSH {
				return nil, fmt.Errorf("ssh_config_id %s 类型 %s 不是 ssh", configID, c.Kind)
			}
			return c, nil
		}
	}
	return nil, fmt.Errorf("ssh_config_id 未找到: %s", configID)
}

// ── 取值辅助 ──────────────────────────────────────────────

func stringField(fields map[string]any, key string) string {
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

func boolField(fields map[string]any, key string, fallback bool) bool {
	if fields == nil {
		return fallback
	}
	v, ok := fields[key]
	if !ok {
		return fallback
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return fallback
}

func intField(fields map[string]any, key string, fallback int) int {
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
