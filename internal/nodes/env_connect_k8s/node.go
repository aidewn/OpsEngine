// env_connect_k8s 节点：从环境配置读取 kubeconfig 并建立 K8s 客户端
// 与 k8s_connect 的区别：kubeconfig YAML 从环境集中维护，不是节点 config 内联

package env_connect_k8s

import (
	"fmt"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node env_connect_k8s 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "env_connect_k8s",
		DisplayName: "环境 · K8s 连接",
		Category:    "environment",
		NodeKind:    core.NodeKindAction,
		Icon:        "☸",
		Description: "从环境配置读取 kubeconfig 并连接 K8s API Server，输出 K8sContext 句柄",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "client", Label: "K8s", PortType: core.PortTypeK8sContext},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "env_select", ID: "environment_id", Label: "环境", Required: true},
			{Type: "env_config_select", ID: "config_id", Label: "K8s 配置",
				Required: true, ConfigKindFilter: string(core.EnvConfigKindK8s)},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 解析环境 → 找 k8s 配置 → 构造 Clientset → Ping
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	envID := strings.TrimSpace(ctx.ConfigString("environment_id"))
	configID := strings.TrimSpace(ctx.ConfigString("config_id"))
	if envID == "" {
		return nil, fmt.Errorf("environment_id 未配置")
	}
	if configID == "" {
		return nil, fmt.Errorf("config_id 未配置")
	}
	envStore := ctx.EnvironmentStore()
	if envStore == nil {
		return nil, fmt.Errorf("引擎未配置 environmentStore")
	}
	env, err := envStore.Get(envID)
	if err != nil {
		return nil, err
	}

	item, err := findK8sConfig(env, configID)
	if err != nil {
		return nil, err
	}

	kubeconfig := stringField(item.Fields, "kubeconfig_yaml")
	contextName := strings.TrimSpace(stringField(item.Fields, "context"))
	namespace := strings.TrimSpace(stringField(item.Fields, "namespace"))
	if strings.TrimSpace(kubeconfig) == "" {
		return nil, fmt.Errorf("K8s 配置缺少 kubeconfig_yaml")
	}

	ctx.Info("解析环境 %s 内 kubeconfig 并连接 K8s API Server...", env.Name)
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
