// env_probe_k8s_pods 节点
// Pure 节点（无 exec 端口）；按 resolve_mode 工作：
//   static  —— 不连远程，从 probe_snapshot 还原 output 端口值
//   dynamic —— 调用本包注册的 Probe 函数实时列 Pod
// 编辑态「探测一次」复用 Probe 函数

package env_probe_k8s_pods

import (
	"context"
	"fmt"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	"OpsEngine/internal/probe"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TypeID 节点与探测函数共用同一 ID
const TypeID = "env_probe_k8s_pods"

func init() {
	engine.Register(&Node{})
	probe.Register(TypeID, Probe)
}

// Node K8s Pod 探测节点
type Node struct{}

// TypeDef 节点元信息
// 输出端口对应 plan §6.2：selected_pod / selected_namespace / names
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      TypeID,
		DisplayName: "环境 · K8s 列 Pod",
		Category:    "environment",
		NodeKind:    core.NodeKindPure,
		Icon:        "☸",
		Description: "列出环境 K8s 集群指定 namespace 内的 Pod；编辑态可点选，运行态按 resolve_mode 还原或重探",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "selected_pod", Label: "selected_pod", PortType: core.PortTypeString},
			{ID: "selected_namespace", Label: "selected_namespace", PortType: core.PortTypeString},
			{ID: "names", Label: "names", PortType: core.PortTypeAny},
			{ID: "count", Label: "count", PortType: core.PortTypeInt},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "env_select", ID: "environment_id", Label: "环境", Required: true},
			{Type: "env_config_select", ID: "config_id", Label: "K8s 配置",
				Required: true, ConfigKindFilter: string(core.EnvConfigKindK8s)},
			{Type: "text", ID: "namespace", Label: "Namespace（留空使用 K8s 配置默认值）"},
			{Type: "text", ID: "label_selector", Label: "Label 选择器（可选）",
				Placeholder: "app=nginx,tier=frontend"},
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
// items[i].Key = pod name；items[i].Meta["namespace"] = pod namespace
func executeStatic(ctx engine.ExecContext) (engine.Outputs, error) {
	snap, _ := ctx.Config("probe_snapshot").(map[string]any)
	if snap == nil {
		return engine.Outputs{
			"selected_pod":       "",
			"selected_namespace": "",
			"names":              []string{},
			"count":              int64(0),
		}, nil
	}
	picked, _ := snap["picked_key"].(string)
	items := snapshotItems(snap["items"])
	names := make([]string, 0, len(items))
	pickedNS := ""
	for _, it := range items {
		names = append(names, it.Key)
		if it.Key == picked {
			pickedNS = stringFromMeta(it.Meta, "namespace")
		}
	}
	return engine.Outputs{
		"selected_pod":       picked,
		"selected_namespace": pickedNS,
		"names":              names,
		"count":              int64(len(items)),
	}, nil
}

// executeDynamic 实时列 Pod；selected_pod 沿用 snapshot，否则取首项兜底
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
		"namespace":      ctx.ConfigString("namespace"),
		"label_selector": ctx.ConfigString("label_selector"),
	}
	res, err := Probe(env, configID, nodeConfig)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(res.Items))
	for _, it := range res.Items {
		names = append(names, it.Key)
	}
	pickedPod := ""
	pickedNS := ""
	if snap, ok := ctx.Config("probe_snapshot").(map[string]any); ok {
		pickedPod, _ = snap["picked_key"].(string)
	}
	// 在最新结果中定位选中 Pod 的 namespace
	for _, it := range res.Items {
		if it.Key == pickedPod {
			pickedNS = stringFromMeta(it.Meta, "namespace")
			break
		}
	}
	// snapshot 失效或缺失时用首项兜底
	if pickedPod == "" && len(res.Items) > 0 {
		pickedPod = res.Items[0].Key
		pickedNS = stringFromMeta(res.Items[0].Meta, "namespace")
	}
	return engine.Outputs{
		"selected_pod":       pickedPod,
		"selected_namespace": pickedNS,
		"names":              names,
		"count":              int64(len(res.Items)),
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

// stringFromMeta 从 ProbeItem.Meta 中安全读 string 字段
func stringFromMeta(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	if v, ok := meta[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Probe K8s Pod 列表探测主体
// 自行解析 kubeconfig → ClientSet → List Pods
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

	// 节点层 namespace 覆盖配置层 namespace；都为空则用 K8sClient 自动解析的默认值
	namespace := strings.TrimSpace(stringField(nodeConfig, "namespace"))
	if namespace == "" {
		namespace = k8sClient.Namespace
	}
	labelSelector := strings.TrimSpace(stringField(nodeConfig, "label_selector"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	list, err := k8sClient.Clientset().CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return probe.ProbeResult{}, fmt.Errorf("List Pods 失败: %w", err)
	}

	items := make([]probe.ProbeItem, 0, len(list.Items))
	for i := range list.Items {
		p := &list.Items[i]
		items = append(items, probe.ProbeItem{
			Key:   p.Name,
			Label: p.Name,
			Meta: map[string]any{
				"namespace": p.Namespace,
				"phase":     string(p.Status.Phase),
				"node":      p.Spec.NodeName,
			},
		})
	}
	return probe.ProbeResult{Items: items}, nil
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

// stringField 从 fields map 中读字符串字段
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
