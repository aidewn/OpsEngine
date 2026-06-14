// k8s.workloads 数据源：复用 env_probe_k8s_workloads 的探测逻辑。
// 每个工作负载容器一项，携带副本数与就绪副本数，便于判断「不可用副本」。

package monitor

import (
	"context"

	probek8s "OpsEngine/internal/nodes/env_probe_k8s_workloads"
	"OpsEngine/internal/probe"
)

// KindK8sWorkloads 标识。
const KindK8sWorkloads = "k8s.workloads"

// K8sWorkload 单个工作负载容器的状态。
type K8sWorkload struct {
	Kind                string `json:"kind"` // Deployment / StatefulSet / DaemonSet
	Namespace           string `json:"namespace"`
	Name                string `json:"name"`
	Container           string `json:"container"`
	Image               string `json:"image"`
	Replicas            int64  `json:"replicas"`
	ReadyReplicas       int64  `json:"ready_replicas"`
	UnavailableReplicas int64  `json:"unavailable_replicas"`
}

// K8sWorkloadsResult 工作负载列表。
type K8sWorkloadsResult struct {
	Workloads []K8sWorkload `json:"workloads"`
}

type k8sWorkloadsSource struct{}

func (k8sWorkloadsSource) Kind() string { return KindK8sWorkloads }

// Collect 参数：namespace（可选，覆盖配置默认命名空间）、include_*（默认全包含）。
func (k8sWorkloadsSource) Collect(_ context.Context, cc CollectContext) (any, error) {
	nodeConfig := map[string]any{
		"namespace":            paramString(cc.Task.Params, "namespace"),
		"include_deployments":  paramBool(cc.Task.Params, "include_deployments", true),
		"include_statefulsets": paramBool(cc.Task.Params, "include_statefulsets", true),
		"include_daemonsets":   paramBool(cc.Task.Params, "include_daemonsets", true),
	}
	res, err := probek8s.Probe(cc.Env, cc.Task.TargetID, nodeConfig)
	if err != nil {
		return nil, err
	}
	return K8sWorkloadsResult{Workloads: mapK8sItems(res.Items)}, nil
}

// mapK8sItems 把 ProbeItem 映射为工作负载结构，并算出不可用副本。
func mapK8sItems(items []probe.ProbeItem) []K8sWorkload {
	out := make([]K8sWorkload, 0, len(items))
	for _, it := range items {
		w := K8sWorkload{}
		if it.Meta != nil {
			w.Kind, _ = it.Meta["kind"].(string)
			w.Namespace, _ = it.Meta["namespace"].(string)
			w.Name, _ = it.Meta["name"].(string)
			w.Container, _ = it.Meta["container"].(string)
			w.Image, _ = it.Meta["current_image"].(string)
			w.Replicas = metaInt64(it.Meta["replicas"])
			w.ReadyReplicas = metaInt64(it.Meta["ready_replicas"])
		}
		if u := w.Replicas - w.ReadyReplicas; u > 0 {
			w.UnavailableReplicas = u
		}
		out = append(out, w)
	}
	return out
}

// metaInt64 兼容 Meta 中数值为 int64 或 float64 的情况。
func metaInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case int:
		return int64(n)
	}
	return 0
}
