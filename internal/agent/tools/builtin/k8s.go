// K8s 只读工具：列 Pod / 列 Workload / 描述 workload。
//
// 列操作直接复用 probe.Run（pod / workload 的 Probe 已经实现）。
// describe 需要直接调 K8s API，故自己拨号一次。

package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"OpsEngine/internal/agent/tools"
	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/probe"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const k8sToolTimeout = 15 * time.Second

// dialK8sForTool 从 K8s 配置构造 Clientset。
func dialK8sForTool(cfg core.EnvConfigItem) (*clients.K8sClient, error) {
	kubeconfig := stringFieldDocker(cfg.Fields, "kubeconfig_yaml")
	cfgContext := strings.TrimSpace(stringFieldDocker(cfg.Fields, "context"))
	cfgNamespace := strings.TrimSpace(stringFieldDocker(cfg.Fields, "namespace"))
	if strings.TrimSpace(kubeconfig) == "" {
		return nil, fmt.Errorf("K8s 配置缺少 kubeconfig_yaml")
	}
	return clients.NewK8sClientFromKubeconfig(kubeconfig, cfgNamespace, cfgContext)
}

// ── k8s_list_pods ──────────────────────────────────────────────

// K8sListPods 列 Pod，复用 env_probe_k8s_pods 的 Probe。
type K8sListPods struct{}

func (K8sListPods) Spec() tools.Spec {
	return tools.Spec{
		Name: "k8s_list_pods",
		Description: "列出当前环境 K8s 集群指定 namespace 的 Pod。返回名字、namespace、phase、节点。" +
			"用于了解服务运行情况，找处于异常 phase（Pending/CrashLoopBackOff）的 Pod。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"namespace":      {Type: "string", Description: "namespace（留空使用配置默认值）。"},
			"label_selector": {Type: "string", Description: "K8s label 选择器，例 app=nginx,tier=frontend（可选）。"},
		},
	}
}

func (K8sListPods) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	cfg, env, err := pickEnvConfig(ctx, core.EnvConfigKindK8s)
	if err != nil {
		return tools.Result{}, err
	}
	res, err := probe.Run("env_probe_k8s_pods", env, cfg.ID, map[string]any{
		"namespace":      argStringOptional(args, "namespace"),
		"label_selector": argStringOptional(args, "label_selector"),
	})
	if err != nil {
		return tools.Result{}, err
	}
	type entry struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Phase     string `json:"phase"`
		Node      string `json:"node"`
	}
	entries := make([]entry, 0, len(res.Items))
	for _, it := range res.Items {
		ns, _ := it.Meta["namespace"].(string)
		phase, _ := it.Meta["phase"].(string)
		node, _ := it.Meta["node"].(string)
		entries = append(entries, entry{Name: it.Key, Namespace: ns, Phase: phase, Node: node})
	}
	data, _ := json.MarshalIndent(entries, "", "  ")
	return tools.Result{
		Output:         tools.TruncateOutput(string(data)),
		DisplaySummary: fmt.Sprintf("k8s_list_pods @ %s（%d 个）", env.Name, len(entries)),
		View:           listView("k8s_pods", fmt.Sprintf("Pod · %s（%d）", env.Name, len(entries)), string(data)),
	}, nil
}

// ── k8s_list_workloads ─────────────────────────────────────────

// K8sListWorkloads 列 Deployment/StatefulSet/DaemonSet，每个容器一个条目。
type K8sListWorkloads struct{}

func (K8sListWorkloads) Spec() tools.Spec {
	return tools.Spec{
		Name: "k8s_list_workloads",
		Description: "列出当前环境 K8s 集群的 Deployment / StatefulSet / DaemonSet 工作负载，" +
			"展开每个容器并返回 image / replicas / ready_replicas。" +
			"用于了解部署形态、找未就绪的工作负载。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"namespace":            {Type: "string", Description: "namespace（留空用配置默认值）。"},
			"include_deployments":  {Type: "boolean", Description: "是否包含 Deployments，默认 true。", Default: true},
			"include_statefulsets": {Type: "boolean", Description: "是否包含 StatefulSets，默认 true。", Default: true},
			"include_daemonsets":   {Type: "boolean", Description: "是否包含 DaemonSets，默认 true。", Default: true},
		},
	}
}

func (K8sListWorkloads) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	cfg, env, err := pickEnvConfig(ctx, core.EnvConfigKindK8s)
	if err != nil {
		return tools.Result{}, err
	}
	res, err := probe.Run("env_probe_k8s_workloads", env, cfg.ID, map[string]any{
		"namespace":            argStringOptional(args, "namespace"),
		"include_deployments":  argBool(args, "include_deployments", true),
		"include_statefulsets": argBool(args, "include_statefulsets", true),
		"include_daemonsets":   argBool(args, "include_daemonsets", true),
	})
	if err != nil {
		return tools.Result{}, err
	}
	// 直接把 ProbeItem.Meta 渲染成 JSON，保留 ready/replicas 字段
	type entry struct {
		Kind          string `json:"kind"`
		Namespace     string `json:"namespace"`
		Name          string `json:"name"`
		Container     string `json:"container"`
		CurrentImage  string `json:"current_image"`
		Replicas      int64  `json:"replicas"`
		ReadyReplicas int64  `json:"ready_replicas"`
	}
	entries := make([]entry, 0, len(res.Items))
	for _, it := range res.Items {
		kind, _ := it.Meta["kind"].(string)
		ns, _ := it.Meta["namespace"].(string)
		name, _ := it.Meta["name"].(string)
		ctr, _ := it.Meta["container"].(string)
		img, _ := it.Meta["current_image"].(string)
		replicas, _ := it.Meta["replicas"].(int64)
		ready, _ := it.Meta["ready_replicas"].(int64)
		entries = append(entries, entry{
			Kind: kind, Namespace: ns, Name: name, Container: ctr,
			CurrentImage: img, Replicas: replicas, ReadyReplicas: ready,
		})
	}
	data, _ := json.MarshalIndent(entries, "", "  ")
	return tools.Result{
		Output:         tools.TruncateOutput(string(data)),
		DisplaySummary: fmt.Sprintf("k8s_list_workloads @ %s（%d 个容器条目）", env.Name, len(entries)),
		View:           listView("k8s_workloads", fmt.Sprintf("工作负载 · %s（%d）", env.Name, len(entries)), string(data)),
	}, nil
}

// ── k8s_describe_pod ───────────────────────────────────────────

// K8sDescribePod 取单个 Pod 的详细状态（含 conditions / containerStatuses）。
type K8sDescribePod struct{}

func (K8sDescribePod) Spec() tools.Spec {
	return tools.Spec{
		Name: "k8s_describe_pod",
		Description: "查看指定 Pod 的详细状态：phase / conditions / containerStatuses（含 restart count / lastTerminationState / 镜像）。" +
			"用于排查 Pod 启动失败、CrashLoop 等。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"name":      {Type: "string", Description: "Pod 名字。", Required: true},
			"namespace": {Type: "string", Description: "namespace（留空使用配置默认值）。"},
		},
	}
}

func (K8sDescribePod) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	name, err := argString(args, "name")
	if err != nil {
		return tools.Result{}, err
	}
	cfg, _, err := pickEnvConfig(ctx, core.EnvConfigKindK8s)
	if err != nil {
		return tools.Result{}, err
	}
	k8sClient, err := dialK8sForTool(cfg)
	if err != nil {
		return tools.Result{}, err
	}
	apiCtx, cancel := context.WithTimeout(context.Background(), k8sToolTimeout)
	defer cancel()

	ns := argStringOptional(args, "namespace")
	if ns == "" {
		ns = k8sClient.Namespace
	}
	pod, err := k8sClient.Clientset().CoreV1().Pods(ns).Get(apiCtx, name, metav1.GetOptions{})
	if err != nil {
		return tools.Result{}, fmt.Errorf("Get Pod 失败: %w", err)
	}
	// 精简输出：只保留排障常看字段
	type containerStatus struct {
		Name         string `json:"name"`
		Image        string `json:"image"`
		Ready        bool   `json:"ready"`
		RestartCount int32  `json:"restart_count"`
		State        string `json:"state"`            // running / waiting / terminated
		Reason       string `json:"reason,omitempty"` // waiting/terminated 时附带
		Message      string `json:"message,omitempty"`
	}
	type podSummary struct {
		Name            string            `json:"name"`
		Namespace       string            `json:"namespace"`
		Node            string            `json:"node"`
		Phase           string            `json:"phase"`
		PodIP           string            `json:"pod_ip"`
		StartTime       string            `json:"start_time,omitempty"`
		Conditions      map[string]string `json:"conditions"`
		ContainerStatus []containerStatus `json:"container_status"`
	}
	conditions := map[string]string{}
	for _, c := range pod.Status.Conditions {
		conditions[string(c.Type)] = string(c.Status)
	}
	cs := make([]containerStatus, 0, len(pod.Status.ContainerStatuses))
	for _, st := range pod.Status.ContainerStatuses {
		entry := containerStatus{
			Name: st.Name, Image: st.Image, Ready: st.Ready, RestartCount: st.RestartCount,
		}
		switch {
		case st.State.Running != nil:
			entry.State = "running"
		case st.State.Waiting != nil:
			entry.State = "waiting"
			entry.Reason = st.State.Waiting.Reason
			entry.Message = st.State.Waiting.Message
		case st.State.Terminated != nil:
			entry.State = "terminated"
			entry.Reason = st.State.Terminated.Reason
			entry.Message = st.State.Terminated.Message
		}
		cs = append(cs, entry)
	}
	startTime := ""
	if pod.Status.StartTime != nil {
		startTime = pod.Status.StartTime.Format(time.RFC3339)
	}
	summary := podSummary{
		Name: pod.Name, Namespace: pod.Namespace, Node: pod.Spec.NodeName,
		Phase: string(pod.Status.Phase), PodIP: pod.Status.PodIP,
		StartTime: startTime, Conditions: conditions, ContainerStatus: cs,
	}
	data, _ := json.MarshalIndent(summary, "", "  ")
	return tools.Result{
		Output:         tools.TruncateOutput(string(data)),
		DisplaySummary: fmt.Sprintf("k8s_describe_pod %s/%s", ns, name),
	}, nil
}

// ── k8s_pod_logs ───────────────────────────────────────────────

// K8sPodLogs 读取指定 Pod 容器的日志尾部，排障刚需。
type K8sPodLogs struct{}

func (K8sPodLogs) Spec() tools.Spec {
	return tools.Spec{
		Name: "k8s_pod_logs",
		Description: "读取指定 Pod 的日志尾部（默认 100 行）。多容器 Pod 需传 container；" +
			"previous=true 读取上一次崩溃前的日志（排查 CrashLoopBackOff 必备）。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"name":      {Type: "string", Description: "Pod 名字。", Required: true},
			"namespace": {Type: "string", Description: "namespace（留空使用配置默认值）。"},
			"container": {Type: "string", Description: "容器名（单容器 Pod 可省略）。"},
			"tail":      {Type: "integer", Description: "日志尾部行数，默认 100，上限 500。", Default: 100},
			"previous":  {Type: "boolean", Description: "读取上一次重启前的日志，默认 false。", Default: false},
		},
	}
}

func (K8sPodLogs) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	name, err := argString(args, "name")
	if err != nil {
		return tools.Result{}, err
	}
	cfg, _, err := pickEnvConfig(ctx, core.EnvConfigKindK8s)
	if err != nil {
		return tools.Result{}, err
	}
	k8sClient, err := dialK8sForTool(cfg)
	if err != nil {
		return tools.Result{}, err
	}
	apiCtx, cancel := context.WithTimeout(context.Background(), k8sToolTimeout)
	defer cancel()

	ns := argStringOptional(args, "namespace")
	if ns == "" {
		ns = k8sClient.Namespace
	}
	tail := int64(argInt(args, "tail", 100))
	if tail < 1 {
		tail = 100
	}
	if tail > 500 {
		tail = 500
	}
	opts := &corev1.PodLogOptions{TailLines: &tail}
	if c := strings.TrimSpace(argStringOptional(args, "container")); c != "" {
		opts.Container = c
	}
	if prev, ok := args["previous"].(bool); ok {
		opts.Previous = prev
	}
	stream, err := k8sClient.Clientset().CoreV1().Pods(ns).GetLogs(strings.TrimSpace(name), opts).Stream(apiCtx)
	if err != nil {
		return tools.Result{}, fmt.Errorf("读取 Pod 日志失败: %w", err)
	}
	defer stream.Close()
	raw, err := io.ReadAll(io.LimitReader(stream, 64*1024)) // 双保险：行数 + 字节数都有上限
	if err != nil {
		return tools.Result{}, fmt.Errorf("读取日志流失败: %w", err)
	}
	header := fmt.Sprintf("Pod %s/%s 日志（尾部 %d 行）:\n", ns, name, tail)
	return tools.Result{
		Output:         tools.TruncateOutput(header + string(raw)),
		DisplaySummary: fmt.Sprintf("k8s_pod_logs %s", name),
	}, nil
}
