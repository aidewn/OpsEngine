// k8s_find_workload 节点
// 在 K8s 集群指定 namespace 中按名称精确查找 Deployment/StatefulSet/DaemonSet
// 把命中工作负载的每个容器展开为 Kind/Name/Container 格式的 workload_ref
// 与 k8s_set_workload_image 的 workload_ref 格式完全对齐

package k8s_find_workload

import (
	"context"
	"fmt"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kindDeployment  = "Deployment"
	kindStatefulSet = "StatefulSet"
	kindDaemonSet   = "DaemonSet"
)

func init() { engine.Register(&Node{}) }

// Node k8s_find_workload 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "k8s_find_workload",
		DisplayName: "K8s · 查找工作负载",
		Category:    "k8s",
		NodeKind:    core.NodeKindAction,
		Icon:        "🔎",
		Description: "在指定 namespace 中按名称精确查找 Deployment/StatefulSet/DaemonSet，输出 workload_ref 列表（Kind/Name/Container）",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "k8s", Label: "K8s", PortType: core.PortTypeK8sContext, Required: true},
			{ID: "name", Label: "名称", PortType: core.PortTypeString},
			{ID: "namespace", Label: "命名空间", PortType: core.PortTypeString},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "workload_ref", Label: "workload_ref", PortType: core.PortTypeString},
			{ID: "workloads", Label: "workload_ref 列表", PortType: core.PortTypeAny},
			{ID: "found", Label: "是否命中", PortType: core.PortTypeBool},
			{ID: "count", Label: "命中数量", PortType: core.PortTypeInt},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "name", Label: "工作负载名称", Placeholder: "myapp"},
			{Type: "text", ID: "namespace", Label: "命名空间", Placeholder: "default"},
			{Type: "toggle", ID: "include_deployments", Label: "包含 Deployment", Default: true},
			{Type: "toggle", ID: "include_statefulsets", Label: "包含 StatefulSet", Default: true},
			{Type: "toggle", ID: "include_daemonsets", Label: "包含 DaemonSet", Default: true},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 在 K8s 集群中精确查找工作负载
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	k8sCli, err := k8sClientFromInput(ctx)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(stringInput(ctx, "name"))
	if name == "" {
		name = strings.TrimSpace(ctx.ConfigString("name"))
	}
	if name == "" {
		return nil, fmt.Errorf("k8s_find_workload 节点的 name 未配置")
	}

	namespace := strings.TrimSpace(stringInput(ctx, "namespace"))
	if namespace == "" {
		namespace = strings.TrimSpace(ctx.ConfigString("namespace"))
	}
	if namespace == "" {
		namespace = k8sCli.Namespace
	}

	includeDeploy := ctx.ConfigBool("include_deployments")
	includeSts := ctx.ConfigBool("include_statefulsets")
	includeDs := ctx.ConfigBool("include_daemonsets")
	// 三个开关都为零值（前端未写入默认值）时，回退全搜
	if !includeDeploy && !includeSts && !includeDs {
		includeDeploy, includeSts, includeDs = true, true, true
	}

	bgCtx, cancel := context.WithCancel(ctx.Context())
	defer cancel()

	apps := k8sCli.Clientset().AppsV1()
	var refs []string

	if includeDeploy {
		d, err := apps.Deployments(namespace).Get(bgCtx, name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("查询 Deployment %s 失败: %w", name, err)
		}
		if err == nil {
			refs = append(refs, containerRefs(kindDeployment, name, d.Spec.Template.Spec.Containers)...)
		}
	}

	if includeSts {
		s, err := apps.StatefulSets(namespace).Get(bgCtx, name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("查询 StatefulSet %s 失败: %w", name, err)
		}
		if err == nil {
			refs = append(refs, containerRefs(kindStatefulSet, name, s.Spec.Template.Spec.Containers)...)
		}
	}

	if includeDs {
		d, err := apps.DaemonSets(namespace).Get(bgCtx, name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("查询 DaemonSet %s 失败: %w", name, err)
		}
		if err == nil {
			refs = append(refs, containerRefs(kindDaemonSet, name, d.Spec.Template.Spec.Containers)...)
		}
	}

	if len(refs) == 0 {
		ctx.Warn("未找到工作负载: name=%s namespace=%s", name, namespace)
		return engine.Outputs{
			"workload_ref": "",
			"workloads":    []string{},
			"found":        false,
			"count":        int64(0),
		}, nil
	}

	ctx.Info("找到 %d 个工作负载引用: name=%s namespace=%s", len(refs), name, namespace)
	return engine.Outputs{
		"workload_ref": refs[0],
		"workloads":    refs,
		"found":        true,
		"count":        int64(len(refs)),
	}, nil
}

// containerRefs 把工作负载的每个容器展开为 Kind/Name/Container 格式引用
func containerRefs(kind, name string, containers []corev1.Container) []string {
	out := make([]string, 0, len(containers))
	for _, c := range containers {
		out = append(out, fmt.Sprintf("%s/%s/%s", kind, name, c.Name))
	}
	return out
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

// stringInput 读字符串输入端口，缺失或类型不匹配返回空串
func stringInput(ctx engine.ExecContext, portID string) string {
	value, ok := ctx.Input(portID)
	if !ok || value == nil {
		return ""
	}
	s, _ := value.(string)
	return s
}
