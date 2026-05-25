// K8s 运行时句柄
// 从 kubeconfig YAML 文本解析，生成 *kubernetes.Clientset
// 句柄可在同一次执行中通过 K8sContext 端口传递，序列化时只暴露安全元数据

package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// K8sClient K8s 运行时句柄
// 内部持有解析后的 REST 配置 + Clientset；K8s client-go 没有显式 Close 概念，所以无需 Close 方法
type K8sClient struct {
	clientset *kubernetes.Clientset
	config    *rest.Config

	Server      string    `json:"server"`
	Namespace   string    `json:"namespace"`
	ContextName string    `json:"context_name"`
	ConnectedAt time.Time `json:"connected_at"`
}

// NewK8sClientFromKubeconfig 从 kubeconfig YAML 文本构造 K8s 客户端
// namespace 为空时默认 "default"；contextName 为空时使用 kubeconfig 当前上下文
func NewK8sClientFromKubeconfig(kubeconfigYAML, namespace, contextName string) (*K8sClient, error) {
	if kubeconfigYAML == "" {
		return nil, fmt.Errorf("kubeconfig 内容为空")
	}

	rawConfig, err := clientcmd.Load([]byte(kubeconfigYAML))
	if err != nil {
		return nil, fmt.Errorf("解析 kubeconfig 失败: %w", err)
	}

	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		if _, ok := rawConfig.Contexts[contextName]; !ok {
			return nil, fmt.Errorf("kubeconfig 中不存在上下文 %q", contextName)
		}
		overrides.CurrentContext = contextName
	}

	clientConfig := clientcmd.NewDefaultClientConfig(*rawConfig, overrides)
	restCfg, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("生成 REST 配置失败: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("构造 K8s Clientset 失败: %w", err)
	}

	if namespace == "" {
		// 优先用 kubeconfig 当前上下文里的 namespace；否则回退 default
		if ns, _, err := clientConfig.Namespace(); err == nil && ns != "" {
			namespace = ns
		} else {
			namespace = "default"
		}
	}

	resolvedContext := contextName
	if resolvedContext == "" {
		resolvedContext = rawConfig.CurrentContext
	}

	return &K8sClient{
		clientset:   clientset,
		config:      restCfg,
		Server:      restCfg.Host,
		Namespace:   namespace,
		ContextName: resolvedContext,
		ConnectedAt: time.Now(),
	}, nil
}

// Clientset 返回底层 K8s 客户端，供后续 k8s_* 操作节点使用
func (c *K8sClient) Clientset() *kubernetes.Clientset {
	if c == nil {
		return nil
	}
	return c.clientset
}

// RESTConfig 返回 REST 配置，供需要自建 dynamic / discovery client 的场景使用
func (c *K8sClient) RESTConfig() *rest.Config {
	if c == nil {
		return nil
	}
	return c.config
}

// Ping 用 namespace get 探活，确认 API Server 可达
func (c *K8sClient) Ping(ctx context.Context) error {
	if c == nil || c.clientset == nil {
		return fmt.Errorf("K8s 客户端未初始化")
	}
	_, err := c.clientset.CoreV1().Namespaces().Get(ctx, c.Namespace, metav1.GetOptions{})
	if err != nil {
		// namespace 不存在不算连不上，只要 API Server 应答即可
		if _, listErr := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1}); listErr != nil {
			return listErr
		}
	}
	return nil
}

// MarshalJSON 只输出安全元数据，不暴露 kubeconfig 内容或 token
func (c *K8sClient) MarshalJSON() ([]byte, error) {
	if c == nil {
		return []byte("null"), nil
	}
	type safeView struct {
		Type        string    `json:"type"`
		Server      string    `json:"server"`
		Namespace   string    `json:"namespace"`
		ContextName string    `json:"context_name"`
		Connected   bool      `json:"connected"`
		ConnectedAt time.Time `json:"connected_at"`
	}
	return json.Marshal(safeView{
		Type:        "K8sContext",
		Server:      c.Server,
		Namespace:   c.Namespace,
		ContextName: c.ContextName,
		Connected:   c.clientset != nil,
		ConnectedAt: c.ConnectedAt,
	})
}
