// env_probe_jenkins_jobs 节点
// Pure 节点（无 exec 端口）；按 resolve_mode 工作：
//   static  —— 不连远程，从 probe_snapshot 还原 output 端口值
//   dynamic —— 调用本包注册的 Probe 函数实时列 job
// 编辑态「探测一次」复用 Probe 函数

package env_probe_jenkins_jobs

import (
	"context"
	"fmt"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	"OpsEngine/internal/probe"
)

// TypeID 节点与探测函数共用同一 ID
const TypeID = "env_probe_jenkins_jobs"

func init() {
	engine.Register(&Node{})
	probe.Register(TypeID, Probe)
}

// Node Jenkins job 探测节点
type Node struct{}

// TypeDef 节点元信息
// 输出端口对应 plan §6.2：selected_job
// 同时输出 names / count 便于下游迭代
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      TypeID,
		DisplayName: "环境 · Jenkins 列 Job",
		Category:    "environment",
		NodeKind:    core.NodeKindPure,
		Icon:        "🛠",
		Description: "列出 Jenkins 指定 folder 下的 job；编辑态可点选，运行态按 resolve_mode 还原或重探",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "selected_job", Label: "selected_job", PortType: core.PortTypeString},
			{ID: "names", Label: "names", PortType: core.PortTypeAny},
			{ID: "count", Label: "count", PortType: core.PortTypeInt},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "env_select", ID: "environment_id", Label: "环境", Required: true},
			{Type: "env_config_select", ID: "config_id", Label: "Jenkins 配置",
				Required: true, ConfigKindFilter: string(core.EnvConfigKindJenkins)},
			{Type: "text", ID: "folder", Label: "Folder 路径（可选）",
				Placeholder: "留空列根目录；嵌套可写 foo/bar"},
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
// items[i].Key = job name；items[i].Label = job name；Meta 含 url/color
func executeStatic(ctx engine.ExecContext) (engine.Outputs, error) {
	snap, _ := ctx.Config("probe_snapshot").(map[string]any)
	if snap == nil {
		return engine.Outputs{
			"selected_job": "",
			"names":        []string{},
			"count":        int64(0),
		}, nil
	}
	picked, _ := snap["picked_key"].(string)
	items := snapshotItems(snap["items"])
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Key)
	}
	return engine.Outputs{
		"selected_job": picked,
		"names":        names,
		"count":        int64(len(items)),
	}, nil
}

// executeDynamic 实时列 job；selected_job 沿用 snapshot，否则取首项兜底
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
		"folder": ctx.ConfigString("folder"),
	}
	res, err := Probe(env, configID, nodeConfig)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(res.Items))
	for _, it := range res.Items {
		names = append(names, it.Key)
	}
	picked := ""
	if snap, ok := ctx.Config("probe_snapshot").(map[string]any); ok {
		picked, _ = snap["picked_key"].(string)
	}
	if picked == "" && len(res.Items) > 0 {
		picked = res.Items[0].Key
	}
	return engine.Outputs{
		"selected_job": picked,
		"names":        names,
		"count":        int64(len(res.Items)),
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

// Probe Jenkins job 探测主体
func Probe(env core.EnvironmentDef, configID string, nodeConfig map[string]any) (probe.ProbeResult, error) {
	item, err := findJenkinsConfig(env, configID)
	if err != nil {
		return probe.ProbeResult{}, err
	}
	baseURL := strings.TrimSpace(stringField(item.Fields, "base_url"))
	user := strings.TrimSpace(stringField(item.Fields, "user"))
	token := stringField(item.Fields, "api_token")
	timeout := intField(item.Fields, "timeout_seconds", 10)

	client, err := clients.NewJenkinsClient(baseURL, user, token, timeout)
	if err != nil {
		return probe.ProbeResult{}, err
	}
	folder := strings.TrimSpace(stringField(nodeConfig, "folder"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobs, err := client.ListJobs(ctx, folder)
	if err != nil {
		return probe.ProbeResult{}, err
	}
	items := make([]probe.ProbeItem, 0, len(jobs))
	for _, j := range jobs {
		items = append(items, probe.ProbeItem{
			Key:   j.Name,
			Label: j.Name,
			Meta: map[string]any{
				"url":   j.URL,
				"color": j.Color,
				"class": j.Class,
			},
		})
	}
	return probe.ProbeResult{Items: items}, nil
}

// findJenkinsConfig 在环境中按 configID 定位 jenkins 配置
func findJenkinsConfig(env core.EnvironmentDef, configID string) (*core.EnvConfigItem, error) {
	for i := range env.Configs {
		c := &env.Configs[i]
		if c.ID == configID {
			if c.Kind != core.EnvConfigKindJenkins {
				return nil, fmt.Errorf("配置 %s 类型 %s 不是 jenkins", configID, c.Kind)
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

func intField(fields map[string]any, key string, fallback int) int {
	if fields == nil {
		return fallback
	}
	v, ok := fields[key]
	if !ok {
		return fallback
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return fallback
}
