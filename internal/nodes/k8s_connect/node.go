// k8s_connect 节点：从粘贴的 kubeconfig YAML 文本建立 K8s 客户端连接
// 不依赖宿主机文件系统或 $KUBECONFIG，工作流自包含

package k8s_connect

import (
	"fmt"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node k8s_connect 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "k8s_connect",
		DisplayName: "K8s 连接",
		Category:    "remote",
		NodeKind:    core.NodeKindAction,
		Icon:        "☸",
		Description: "从粘贴的 kubeconfig YAML 文本建立 K8s 客户端，输出 K8sContext",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "client", Label: "K8s", PortType: core.PortTypeK8sContext},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "textarea", ID: "kubeconfig", Label: "kubeconfig YAML",
				Placeholder: "粘贴完整 kubeconfig 内容（含 clusters / users / contexts）", Required: true},
			{Type: "text", ID: "context_name", Label: "Context 名（可选）",
				Placeholder: "留空则使用 kubeconfig 当前上下文"},
			{Type: "text", ID: "namespace", Label: "默认 Namespace（可选）",
				Placeholder: "留空则继承上下文中的 namespace，否则 default"},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 解析 kubeconfig → 构造 Clientset → ping API Server 验证可达
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	kubeconfig := ctx.ConfigString("kubeconfig")
	contextName := strings.TrimSpace(ctx.ConfigString("context_name"))
	namespace := strings.TrimSpace(ctx.ConfigString("namespace"))

	if strings.TrimSpace(kubeconfig) == "" {
		return nil, fmt.Errorf("k8s_connect 节点的 kubeconfig 未配置")
	}

	ctx.Info("解析 kubeconfig 并连接 K8s API Server...")

	k8sClient, err := clients.NewK8sClientFromKubeconfig(kubeconfig, namespace, contextName)
	if err != nil {
		return nil, err
	}

	if err := k8sClient.Ping(ctx.Context()); err != nil {
		return nil, fmt.Errorf("K8s API Server 不可达: %w", err)
	}

	ctx.Info("K8s 连接成功: server=%s namespace=%s context=%s",
		k8sClient.Server, k8sClient.Namespace, k8sClient.ContextName)

	return engine.Outputs{
		"client": k8sClient,
	}, nil
}
