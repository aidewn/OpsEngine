// env_probe_k8s_workloads 节点
// Pure + Probe；列出指定 ns 下的 Deployment / StatefulSet / DaemonSet
// 把每个 workload 的「每个容器」展平为一项，方便下游 set_workload_image 直接点选要改的容器
//
// Items 形态：
//
//	key   = "<kind>/<name>/<container>"   例 "Deployment/myapp/api"
//	label = "<kind> · <name> · <container>  (image: nginx:1.19)"
//	meta  = {kind, name, container, current_image, replicas, ready_replicas, namespace}

package env_probe_k8s_workloads

import (
	"context"
	"fmt"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	"OpsEngine/internal/probe"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TypeID 节点与探测函数共用同一 ID
const TypeID = "env_probe_k8s_workloads"

func init() {
	engine.Register(&Node{})
	probe.Register(TypeID, Probe)
}

// Node K8s 工作负载探测节点
type Node struct{}

// TypeDef 节点元信息
// 输出 selected_workload / workloads / count；selected_workload 形如 Deployment/myapp/api
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      TypeID,
		DisplayName: "环境 · K8s 列工作负载",
		Category:    "environment",
		NodeKind:    core.NodeKindPure,
		Icon:        "☸",
		Description: "列出指定 ns 下 Deployment/StatefulSet/DaemonSet 的容器；编辑态点选某容器，运行态按 resolve_mode 还原或重探",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "selected_workload", Label: "selected_workload", PortType: core.PortTypeString},
			{ID: "workloads", Label: "workloads", PortType: core.PortTypeAny},
			{ID: "count", Label: "count", PortType: core.PortTypeInt},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "env_select", ID: "environment_id", Label: "环境", Required: true},
			{Type: "env_config_select", ID: "config_id", Label: "K8s 配置",
				Required: true, ConfigKindFilter: string(core.EnvConfigKindK8s)},
			{Type: "text", ID: "namespace", Label: "Namespace（留空使用 K8s 配置默认值）"},
			{Type: "toggle", ID: "include_deployments", Label: "包含 Deployment", Default: true},
			{Type: "toggle", ID: "include_statefulsets", Label: "包含 StatefulSet", Default: true},
			{Type: "toggle", ID: "include_daemonsets", Label: "包含 DaemonSet", Default: true},
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
func executeStatic(ctx engine.ExecContext) (engine.Outputs, error) {
	snap, _ := ctx.Config("probe_snapshot").(map[string]any)
	if snap == nil {
		return engine.Outputs{
			"selected_workload": "",
			"workloads":         []string{},
			"count":             int64(0),
		}, nil
	}
	picked, _ := snap["picked_key"].(string)
	items := snapshotItems(snap["items"])
	keys := make([]string, 0, len(items))
	for _, it := range items {
		keys = append(keys, it.Key)
	}
	return engine.Outputs{
		"selected_workload": picked,
		"workloads":         keys,
		"count":             int64(len(keys)),
	}, nil
}

// executeDynamic 实时列；picked 沿用 snapshot，缺失取首项兜底
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
		"namespace":           ctx.ConfigString("namespace"),
		"include_deployments": ctx.ConfigBool("include_deployments"),
		"include_statefulsets": ctx.ConfigBool("include_statefulsets"),
		"include_daemonsets":  ctx.ConfigBool("include_daemonsets"),
	}
	res, err := Probe(env, configID, nodeConfig)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(res.Items))
	for _, it := range res.Items {
		keys = append(keys, it.Key)
	}
	picked := ""
	if snap, ok := ctx.Config("probe_snapshot").(map[string]any); ok {
		picked, _ = snap["picked_key"].(string)
	}
	if picked == "" && len(keys) > 0 {
		picked = keys[0]
	}
	return engine.Outputs{
		"selected_workload": picked,
		"workloads":         keys,
		"count":             int64(len(keys)),
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

// Probe 列工作负载主体：三类 List 各调一次，按 include_* 开关过滤
// 三种 List 默认全开（与 TypeDef 的 toggle 默认值一致）；用户全关时返回空集
func Probe(env core.EnvironmentDef, configID string, nodeConfig map[string]any) (probe.ProbeResult, error) {
	item, err := findK8sConfig(env, configID)
	if err != nil {
		return probe.ProbeResult{}, err
	}
	kubeconfig := stringField(item.Fields, "kubeconfig_yaml")
	cfgContext := strings.TrimSpace(stringField(item.Fields, "context"))
	cfgNamespace := strings.TrimSpace(stringField(item.Fields, "namespace"))
	if strings.TrimSpace(kubeconfig) == "" {
		return probe.ProbeResult{}, fmt.Errorf("K8s 配置缺少 kubeconfig_yaml")
	}

	k8sClient, err := clients.NewK8sClientFromKubeconfig(kubeconfig, cfgNamespace, cfgContext)
	if err != nil {
		return probe.ProbeResult{}, err
	}

	namespace := strings.TrimSpace(stringField(nodeConfig, "namespace"))
	if namespace == "" {
		namespace = k8sClient.Namespace
	}
	// toggle 默认 true 由 ConfigSchema 保障；这里读不到（如手工调用 Probe）就走「全包含」
	includeDeploy := boolField(nodeConfig, "include_deployments", true)
	includeSts := boolField(nodeConfig, "include_statefulsets", true)
	includeDs := boolField(nodeConfig, "include_daemonsets", true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	apps := k8sClient.Clientset().AppsV1()
	var items []probe.ProbeItem

	if includeDeploy {
		list, err := apps.Deployments(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return probe.ProbeResult{}, fmt.Errorf("List Deployments 失败: %w", err)
		}
		for i := range list.Items {
			d := &list.Items[i]
			items = append(items, flattenContainers(
				"Deployment", d.Namespace, d.Name,
				d.Spec.Template.Spec.Containers,
				int64(d.Status.Replicas), int64(d.Status.ReadyReplicas),
			)...)
		}
	}
	if includeSts {
		list, err := apps.StatefulSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return probe.ProbeResult{}, fmt.Errorf("List StatefulSets 失败: %w", err)
		}
		for i := range list.Items {
			s := &list.Items[i]
			items = append(items, flattenContainers(
				"StatefulSet", s.Namespace, s.Name,
				s.Spec.Template.Spec.Containers,
				int64(s.Status.Replicas), int64(s.Status.ReadyReplicas),
			)...)
		}
	}
	if includeDs {
		list, err := apps.DaemonSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return probe.ProbeResult{}, fmt.Errorf("List DaemonSets 失败: %w", err)
		}
		for i := range list.Items {
			d := &list.Items[i]
			items = append(items, flattenContainers(
				"DaemonSet", d.Namespace, d.Name,
				d.Spec.Template.Spec.Containers,
				int64(d.Status.DesiredNumberScheduled), int64(d.Status.NumberReady),
			)...)
		}
	}

	return probe.ProbeResult{Items: items}, nil
}

// flattenContainers 把一个 workload 的容器列表展平为多个 ProbeItem
// 每项含 kind/name/container/current_image 等元数据，便于下游 set_image 直接消费
func flattenContainers(
	kind, namespace, name string,
	containers []corev1.Container,
	replicas, readyReplicas int64,
) []probe.ProbeItem {
	out := make([]probe.ProbeItem, 0, len(containers))
	for _, c := range containers {
		key := fmt.Sprintf("%s/%s/%s", kind, name, c.Name)
		label := fmt.Sprintf("%s · %s · %s  (image: %s)", kind, name, c.Name, c.Image)
		out = append(out, probe.ProbeItem{
			Key:   key,
			Label: label,
			Meta: map[string]any{
				"kind":            kind,
				"namespace":       namespace,
				"name":            name,
				"container":       c.Name,
				"current_image":   c.Image,
				"replicas":        replicas,
				"ready_replicas":  readyReplicas,
			},
		})
	}
	return out
}

// findK8sConfig 在环境中按 configID 定位 k8s 配置
func findK8sConfig(env core.EnvironmentDef, configID string) (*core.EnvConfigItem, error) {
	for i := range env.Configs {
		c := &env.Configs[i]
		if c.ID == configID {
			if c.Kind != core.EnvConfigKindK8s {
				return nil, fmt.Errorf("配置 %s 类型 %s 不是 k8s", configID, c.Kind)
			}
			return c, nil
		}
	}
	return nil, fmt.Errorf("配置未找到: %s", configID)
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
