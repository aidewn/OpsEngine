// k8s_set_workload_image 节点
// 通过 strategic merge patch 改 Deployment / StatefulSet / DaemonSet 的某个容器镜像
// 等价：kubectl set image <kind>/<name> <container>=<image> -n <ns>
//
// workload_ref 编码：<Kind>/<name>/<container>（与 env_probe_k8s_workloads 的 selected_workload 对齐）
// namespace：从 K8sContext.Namespace 拿（k8s_connect 已解析 kubeconfig 当前 ns）
//
// 可选 wait_for_rollout：开启后轮询 status 直到 readyReplicas 满足目标，或超时报错
// 轮询间隔 2s，与一般 kubectl rollout status 同步频率相当

package k8s_set_workload_image

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const (
	pollInterval        = 2 * time.Second
	defaultTimeoutSec   = 300
	maxTimeoutSec       = 3600
	minTimeoutSec       = 5
	kindDeployment      = "Deployment"
	kindStatefulSet     = "StatefulSet"
	kindDaemonSet       = "DaemonSet"
)

func init() { engine.Register(&Node{}) }

// Node k8s_set_workload_image 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	minTo, maxTo := int64(minTimeoutSec), int64(maxTimeoutSec)
	return core.NodeTypeDef{
		TypeID:      "k8s_set_workload_image",
		DisplayName: "K8s · 修改工作负载镜像",
		Category:    "k8s",
		NodeKind:    core.NodeKindAction,
		Icon:        "🔄",
		Description: "改 Deployment/StatefulSet/DaemonSet 指定容器的镜像；可选等待 rollout 完成",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "k8s", Label: "K8s", PortType: core.PortTypeK8sContext, Required: true},
			{ID: "workload_ref", Label: "workload_ref", PortType: core.PortTypeString, Required: true},
			{ID: "image", Label: "image", PortType: core.PortTypeString, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "namespace",
				Label: "Namespace（留空使用 K8s context 默认值）"},
			{Type: "toggle", ID: "wait_for_rollout",
				Label: "等待 rollout 完成", Default: true},
			{Type: "number", ID: "timeout_seconds",
				Label: "超时（秒）", Min: &minTo, Max: &maxTo, Default: int64(defaultTimeoutSec)},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 主流程
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	k8sCli, err := k8sClientFromInput(ctx)
	if err != nil {
		return nil, err
	}
	workloadRef, err := inputString(ctx, "workload_ref")
	if err != nil {
		return nil, err
	}
	image, err := inputString(ctx, "image")
	if err != nil {
		return nil, err
	}
	kind, name, container, err := parseWorkloadRef(workloadRef)
	if err != nil {
		return nil, err
	}

	namespace := strings.TrimSpace(ctx.ConfigString("namespace"))
	if namespace == "" {
		namespace = k8sCli.Namespace
	}
	if namespace == "" {
		return nil, fmt.Errorf("namespace 解析为空；请在 K8s 配置或节点 config 填写")
	}
	waitRollout := ctx.ConfigBool("wait_for_rollout")
	timeoutSec := int(ctx.ConfigInt("timeout_seconds"))
	if timeoutSec <= 0 {
		timeoutSec = defaultTimeoutSec
	}

	ctx.Info("kubectl set image %s/%s %s=%s -n %s", kind, name, container, image, namespace)

	patchBody, err := buildContainerImagePatch(container, image)
	if err != nil {
		return nil, err
	}

	cs := k8sCli.Clientset()
	if err := patchWorkload(ctx.Context(), cs, kind, namespace, name, patchBody); err != nil {
		return nil, err
	}

	if !waitRollout {
		ctx.Info("已提交 patch，不等待 rollout")
		return engine.Outputs{}, nil
	}

	ctx.Info("等待 rollout 完成（最长 %ds）...", timeoutSec)
	waitCtx, cancel := context.WithTimeout(ctx.Context(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	if err := waitForRollout(waitCtx, cs, kind, namespace, name, image, container, ctx.Info); err != nil {
		return nil, err
	}
	ctx.Info("rollout 完成: %s/%s", kind, name)
	return engine.Outputs{}, nil
}

// parseWorkloadRef 解析 "<kind>/<name>/<container>"；任一段缺失或 kind 非法均报错
func parseWorkloadRef(ref string) (kind, name, container string, err error) {
	parts := strings.Split(strings.TrimSpace(ref), "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("workload_ref 格式应为 <Kind>/<name>/<container>，得到: %q", ref)
	}
	kind = parts[0]
	switch kind {
	case kindDeployment, kindStatefulSet, kindDaemonSet:
	default:
		return "", "", "", fmt.Errorf("不支持的 Kind: %s（仅支持 Deployment/StatefulSet/DaemonSet）", kind)
	}
	return kind, parts[1], parts[2], nil
}

// buildContainerImagePatch 构造 strategic merge patch JSON：
//
//	{ "spec": { "template": { "spec": { "containers": [ {"name": <c>, "image": <img>} ] } } } }
//
// strategic merge 会按 containers[].name 做 key 匹配，仅修改目标容器的 image
func buildContainerImagePatch(container, image string) ([]byte, error) {
	patch := map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []map[string]any{
						{"name": container, "image": image},
					},
				},
			},
		},
	}
	return json.Marshal(patch)
}

// patchWorkload 按 kind 路由到对应的 typed client；统一用 StrategicMergePatchType
func patchWorkload(ctx context.Context, cs *kubernetes.Clientset, kind, namespace, name string, body []byte) error {
	apps := cs.AppsV1()
	switch kind {
	case kindDeployment:
		_, err := apps.Deployments(namespace).Patch(ctx, name, types.StrategicMergePatchType, body, metav1.PatchOptions{})
		return wrapPatchErr(kind, name, err)
	case kindStatefulSet:
		_, err := apps.StatefulSets(namespace).Patch(ctx, name, types.StrategicMergePatchType, body, metav1.PatchOptions{})
		return wrapPatchErr(kind, name, err)
	case kindDaemonSet:
		_, err := apps.DaemonSets(namespace).Patch(ctx, name, types.StrategicMergePatchType, body, metav1.PatchOptions{})
		return wrapPatchErr(kind, name, err)
	}
	return fmt.Errorf("不支持的 Kind: %s", kind)
}

func wrapPatchErr(kind, name string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("Patch %s/%s 失败: %w", kind, name, err)
}

// waitForRollout 轮询 status，直到目标容器镜像生效且 ready 副本数满足
//   - Deployment / StatefulSet：ObservedGeneration ≥ Generation 且 ReadyReplicas == Replicas（且 == Spec.Replicas，若设置）
//   - DaemonSet：NumberReady == DesiredNumberScheduled
// 同时校验 spec.template 中目标容器的镜像已更新为期望值（防止脏 patch 误判）
func waitForRollout(
	ctx context.Context,
	cs *kubernetes.Clientset,
	kind, namespace, name, expectedImage, container string,
	log func(string, ...any),
) error {
	apps := cs.AppsV1()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	first := true

	for {
		if !first {
			select {
			case <-ctx.Done():
				return fmt.Errorf("等待 rollout 超时或被取消: %w", ctx.Err())
			case <-ticker.C:
			}
		}
		first = false

		switch kind {
		case kindDeployment:
			d, err := apps.Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("Get Deployment %s 失败: %w", name, err)
			}
			if !containerImageMatches(d.Spec.Template.Spec.Containers, container, expectedImage) {
				log("镜像尚未生效到 spec.template，继续等待...")
				continue
			}
			if d.Status.ObservedGeneration < d.Generation {
				log("ObservedGeneration=%d < Generation=%d", d.Status.ObservedGeneration, d.Generation)
				continue
			}
			desired := int32(0)
			if d.Spec.Replicas != nil {
				desired = *d.Spec.Replicas
			} else {
				desired = d.Status.Replicas
			}
			if d.Status.UpdatedReplicas >= desired && d.Status.ReadyReplicas >= desired && d.Status.UnavailableReplicas == 0 {
				return nil
			}
			log("Deployment %s ready=%d/%d updated=%d unavailable=%d",
				name, d.Status.ReadyReplicas, desired, d.Status.UpdatedReplicas, d.Status.UnavailableReplicas)

		case kindStatefulSet:
			s, err := apps.StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("Get StatefulSet %s 失败: %w", name, err)
			}
			if !containerImageMatches(s.Spec.Template.Spec.Containers, container, expectedImage) {
				continue
			}
			if s.Status.ObservedGeneration < s.Generation {
				continue
			}
			desired := int32(0)
			if s.Spec.Replicas != nil {
				desired = *s.Spec.Replicas
			} else {
				desired = s.Status.Replicas
			}
			if s.Status.ReadyReplicas >= desired && s.Status.UpdatedReplicas >= desired {
				return nil
			}
			log("StatefulSet %s ready=%d/%d updated=%d",
				name, s.Status.ReadyReplicas, desired, s.Status.UpdatedReplicas)

		case kindDaemonSet:
			d, err := apps.DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("Get DaemonSet %s 失败: %w", name, err)
			}
			if !containerImageMatches(d.Spec.Template.Spec.Containers, container, expectedImage) {
				continue
			}
			if d.Status.ObservedGeneration < d.Generation {
				continue
			}
			if d.Status.NumberReady >= d.Status.DesiredNumberScheduled &&
				d.Status.UpdatedNumberScheduled >= d.Status.DesiredNumberScheduled {
				return nil
			}
			log("DaemonSet %s ready=%d/%d updated=%d",
				name, d.Status.NumberReady, d.Status.DesiredNumberScheduled, d.Status.UpdatedNumberScheduled)
		}
	}
}

// containerImageMatches 在 spec.template.containers 中找名为 container 的项，判断 image 是否已更新
func containerImageMatches(containers []corev1.Container, container, expectedImage string) bool {
	for _, c := range containers {
		if c.Name == container {
			return c.Image == expectedImage
		}
	}
	return false
}

// inputString 从输入端口读取字符串值
func inputString(ctx engine.ExecContext, port string) (string, error) {
	v, ok := ctx.Input(port)
	if !ok || v == nil {
		return "", fmt.Errorf("输入端口 %s 未连接", port)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("输入端口 %s 类型不是 string，得到 %T", port, v)
	}
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("输入端口 %s 值为空字符串", port)
	}
	return s, nil
}

// k8sClientFromInput 从 k8s 输入端口取 K8sContext 句柄
func k8sClientFromInput(ctx engine.ExecContext) (*clients.K8sClient, error) {
	v, ok := ctx.Input("k8s")
	if !ok || v == nil {
		return nil, fmt.Errorf("k8s 输入端口未连接 K8sContext")
	}
	c, ok := v.(*clients.K8sClient)
	if !ok {
		return nil, fmt.Errorf("k8s 输入端口类型不是 *K8sClient，得到 %T", v)
	}
	return c, nil
}
