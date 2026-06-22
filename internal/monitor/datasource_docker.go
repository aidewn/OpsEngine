// docker.containers 数据源：复用 env_probe_docker_containers 的探测逻辑（dial+list+close）。

package monitor

import (
	"context"

	"OpsEngine/internal/core"
	probedocker "OpsEngine/internal/nodes/env_probe_docker_containers"
	"OpsEngine/internal/probe"
)

// KindDockerContainers 标识。
const KindDockerContainers = "docker.containers"

// DockerContainer 单个容器的状态。
type DockerContainer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image"`
	State  string `json:"state"`  // running / exited / ...
	Status string `json:"status"` // 人类可读，如 "Up 3 hours"
}

// DockerContainersResult 容器列表。
type DockerContainersResult struct {
	Containers []DockerContainer `json:"containers"`
}

type dockerContainersSource struct{}

func (dockerContainersSource) SourceKind() string { return core.MonitorSourceKindBuiltin }
func (dockerContainersSource) Kind() string       { return KindDockerContainers }

// Collect 参数：all（含已停止容器，默认 false）、filter_name（子串过滤，可选）。
func (dockerContainersSource) Collect(_ context.Context, cc CollectContext) (any, error) {
	nodeConfig := map[string]any{
		"all":         paramBool(cc.Task.Params, "all", false),
		"filter_name": paramString(cc.Task.Params, "filter_name"),
	}
	res, err := probedocker.Probe(cc.Env, cc.Task.TargetID, nodeConfig)
	if err != nil {
		return nil, err
	}
	return DockerContainersResult{Containers: mapDockerItems(res.Items)}, nil
}

// mapDockerItems 把 ProbeItem 映射为容器状态结构。
func mapDockerItems(items []probe.ProbeItem) []DockerContainer {
	out := make([]DockerContainer, 0, len(items))
	for _, it := range items {
		c := DockerContainer{ID: it.Key, Name: it.Label}
		if it.Meta != nil {
			c.Image, _ = it.Meta["image"].(string)
			c.State, _ = it.Meta["state"].(string)
			c.Status, _ = it.Meta["status"].(string)
		}
		out = append(out, c)
	}
	return out
}
