// docker_ps 节点：列出远端 Docker daemon 上的容器
// 输出 JSON 数组（精简字段），方便下游脚本节点解析

package docker_ps

import (
	"encoding/json"
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

// 精简后的容器条目，避免把完整的 container.Summary（含 Mounts、NetworkSettings 等大字段）外泄
type containerEntry struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Image  string   `json:"image"`
	State  string   `json:"state"`
	Status string   `json:"status"`
	Labels []string `json:"labels,omitempty"`
}

func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "docker_ps",
		DisplayName: "Docker 列出容器",
		Category:    "docker",
		NodeKind:    core.NodeKindAction,
		Icon:        "📋",
		Description: "列出远端 Docker daemon 上的容器，输出 JSON 数组",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "client", Label: "Docker", PortType: core.PortTypeDockerContext, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "containers", Label: "JSON", PortType: core.PortTypeString},
			{ID: "count", Label: "Count", PortType: core.PortTypeInt},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "toggle", ID: "all", Label: "包含已停止容器", Default: false},
			{Type: "text", ID: "filter_name", Label: "name 过滤（可选，子串匹配）"},
			{Type: "text", ID: "filter_label", Label: "label 过滤（可选，key=value）"},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	client, err := dockerClientFromInput(ctx)
	if err != nil {
		return nil, err
	}

	args := filters.NewArgs()
	if name := strings.TrimSpace(ctx.ConfigString("filter_name")); name != "" {
		args.Add("name", name)
	}
	if label := strings.TrimSpace(ctx.ConfigString("filter_label")); label != "" {
		args.Add("label", label)
	}

	summaries, err := client.API().ContainerList(ctx.Context(), container.ListOptions{
		All:     ctx.ConfigBool("all"),
		Filters: args,
	})
	if err != nil {
		return nil, fmt.Errorf("docker_ps: ContainerList 失败: %w", err)
	}

	entries := make([]containerEntry, 0, len(summaries))
	for _, s := range summaries {
		entries = append(entries, containerEntry{
			ID:     s.ID,
			Name:   firstName(s.Names),
			Image:  s.Image,
			State:  s.State,
			Status: s.Status,
			Labels: labelsToSlice(s.Labels),
		})
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("docker_ps: 序列化结果失败: %w", err)
	}
	ctx.Info("命中 %d 个容器", len(entries))

	return engine.Outputs{
		"containers": string(data),
		"count":      int64(len(entries)),
	}, nil
}

func firstName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

func labelsToSlice(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
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
