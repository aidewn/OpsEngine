// docker_run 节点的多行字段解析辅助
// env / labels / ports 全部用"一行一条"的简单文本格式，避免引入复杂控件

package docker_run

import (
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
)

// parseEnv 把多行 KEY=VALUE 文本解析成 SDK 需要的 []string
// 忽略空行与 # 开头的注释行
func parseEnv(text string) ([]string, error) {
	var out []string
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "=") {
			return nil, fmt.Errorf("env 第 %d 行缺少 '='：%q", i+1, line)
		}
		out = append(out, line)
	}
	return out, nil
}

// parseLabels 把多行 key=value 文本解析成 map
func parseLabels(text string) (map[string]string, error) {
	out := make(map[string]string)
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			return nil, fmt.Errorf("labels 第 %d 行缺少 '='：%q", i+1, line)
		}
		out[strings.TrimSpace(line[:eq])] = strings.TrimSpace(line[eq+1:])
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// parsePorts 把多行端口映射文本解析成 ExposedPorts + PortBindings
// 支持格式：HOST:CONTAINER 或 HOST:CONTAINER/proto（tcp/udp）
func parsePorts(text string) (nat.PortSet, nat.PortMap, error) {
	var specs []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		specs = append(specs, line)
	}
	if len(specs) == 0 {
		return nil, nil, nil
	}
	exposed, bindings, err := nat.ParsePortSpecs(specs)
	if err != nil {
		return nil, nil, fmt.Errorf("ports 解析失败: %w", err)
	}
	return exposed, bindings, nil
}

// restartPolicy 从字符串映射到 container.RestartPolicy
func restartPolicy(name string) container.RestartPolicy {
	switch strings.TrimSpace(name) {
	case "always":
		return container.RestartPolicy{Name: container.RestartPolicyAlways}
	case "on-failure":
		return container.RestartPolicy{Name: container.RestartPolicyOnFailure}
	case "unless-stopped":
		return container.RestartPolicy{Name: container.RestartPolicyUnlessStopped}
	default:
		return container.RestartPolicy{Name: container.RestartPolicyDisabled}
	}
}
