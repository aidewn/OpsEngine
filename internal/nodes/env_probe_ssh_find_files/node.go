// env_probe_ssh_find_files 节点
// Pure 节点（无 exec 端口）；按 resolve_mode 工作：
//   static  —— 不连远程，从 probe_snapshot 还原 output 端口值
//   dynamic —— 调用本包注册的 Probe 函数实时正则搜索
// 编辑态「探测一次」复用 Probe 函数，与 ssh_list_dir 同构

package env_probe_ssh_find_files

import (
	"fmt"
	"regexp"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	"OpsEngine/internal/probe"
)

// TypeID 节点与探测函数共用同一 ID
const TypeID = "env_probe_ssh_find_files"

const (
	defaultStartDir = "/"
	defaultDepth    = 5
)

func init() {
	engine.Register(&Node{})
	probe.Register(TypeID, Probe)
}

// Node 远程文件正则搜索探测
type Node struct{}

// TypeDef 节点元信息
// 输出端口对应 plan §6.2：selected_path / paths / first_path
// 同时保留 count，便于下游判断是否命中
func (Node) TypeDef() core.NodeTypeDef {
	minDepth, maxDepth := int64(1), int64(32)
	return core.NodeTypeDef{
		TypeID:      TypeID,
		DisplayName: "环境 · SSH 搜索文件",
		Category:    "environment",
		NodeKind:    core.NodeKindPure,
		Icon:        "🔎",
		Description: "通过环境内 SSH 配置按正则搜索远程文件；编辑态可点选，运行态按 resolve_mode 还原或重探",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "selected_path", Label: "selected_path", PortType: core.PortTypeString},
			{ID: "paths", Label: "paths", PortType: core.PortTypeAny},
			{ID: "first_path", Label: "first", PortType: core.PortTypeString},
			{ID: "count", Label: "count", PortType: core.PortTypeInt},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "env_select", ID: "environment_id", Label: "环境", Required: true},
			{Type: "env_config_select", ID: "config_id", Label: "SSH 配置",
				Required: true, ConfigKindFilter: string(core.EnvConfigKindSSH)},
			{Type: "text", ID: "pattern", Label: "文件名正则", Required: true,
				Placeholder: `^.*\.log$`},
			{Type: "text", ID: "start_dir", Label: "起始目录",
				Placeholder: "/", Default: defaultStartDir},
			{Type: "number", ID: "max_depth", Label: "最大深度",
				Min: &minDepth, Max: &maxDepth, Default: int64(defaultDepth)},
			{Type: "toggle", ID: "case_sensitive", Label: "区分大小写", Default: true},
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
			"selected_path": "",
			"paths":         []string{},
			"first_path":    "",
			"count":         int64(0),
		}, nil
	}
	picked, _ := snap["picked_key"].(string)
	items := snapshotItems(snap["items"])
	paths := make([]string, 0, len(items))
	for _, it := range items {
		paths = append(paths, it.Key)
	}
	first := ""
	if len(paths) > 0 {
		first = paths[0]
	}
	return engine.Outputs{
		"selected_path": picked,
		"paths":         paths,
		"first_path":    first,
		"count":         int64(len(paths)),
	}, nil
}

// executeDynamic 实时探测；picked_key 沿用 snapshot，否则取 first 兜底
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
		"pattern":        ctx.ConfigString("pattern"),
		"start_dir":      ctx.ConfigString("start_dir"),
		"max_depth":      ctx.ConfigInt("max_depth"),
		"case_sensitive": ctx.ConfigBool("case_sensitive"),
	}
	res, err := Probe(env, configID, nodeConfig)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(res.Items))
	for _, it := range res.Items {
		paths = append(paths, it.Key)
	}
	picked := ""
	if snap, ok := ctx.Config("probe_snapshot").(map[string]any); ok {
		picked, _ = snap["picked_key"].(string)
	}
	first := ""
	if len(paths) > 0 {
		first = paths[0]
	}
	if picked == "" {
		picked = first
	}
	return engine.Outputs{
		"selected_path": picked,
		"paths":         paths,
		"first_path":    first,
		"count":         int64(len(paths)),
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

// Probe 正则搜索探测主体；复用 clients.WalkSFTPMatch
func Probe(env core.EnvironmentDef, configID string, nodeConfig map[string]any) (probe.ProbeResult, error) {
	item, err := findSSHConfig(env, configID)
	if err != nil {
		return probe.ProbeResult{}, err
	}
	pattern := strings.TrimSpace(stringField(nodeConfig, "pattern"))
	if pattern == "" {
		return probe.ProbeResult{}, fmt.Errorf("pattern 未配置")
	}
	if !boolField(nodeConfig, "case_sensitive", true) {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return probe.ProbeResult{}, fmt.Errorf("正则编译失败: %w", err)
	}
	startDir := strings.TrimSpace(stringField(nodeConfig, "start_dir"))
	if startDir == "" {
		startDir = defaultStartDir
	}
	depth := intField(nodeConfig, "max_depth", defaultDepth)

	host := stringField(item.Fields, "host")
	user := stringField(item.Fields, "user")
	password := stringField(item.Fields, "password")
	port := intField(item.Fields, "port", 22)
	timeout := intField(item.Fields, "timeout_seconds", 10)
	if host == "" || user == "" || password == "" {
		return probe.ProbeResult{}, fmt.Errorf("SSH 配置缺少 host/user/password")
	}
	client, err := clients.DialLinuxSsh(host, port, user, password, timeout)
	if err != nil {
		return probe.ProbeResult{}, err
	}
	defer client.Close()

	// 编辑态探测无 context；传 nil 表示不可取消
	matches, err := clients.WalkSFTPMatch(nil, client, startDir, re, depth, nil)
	if err != nil {
		return probe.ProbeResult{}, err
	}
	items := make([]probe.ProbeItem, 0, len(matches))
	for _, p := range matches {
		// 用 basename 当 label；key 是完整路径，下游直接消费
		base := p
		if idx := strings.LastIndex(p, "/"); idx >= 0 && idx+1 < len(p) {
			base = p[idx+1:]
		}
		items = append(items, probe.ProbeItem{
			Key:   p,
			Label: base,
		})
	}
	return probe.ProbeResult{Items: items}, nil
}

// findSSHConfig 在环境中按 configID 定位 ssh 配置
func findSSHConfig(env core.EnvironmentDef, configID string) (*core.EnvConfigItem, error) {
	for i := range env.Configs {
		c := &env.Configs[i]
		if c.ID == configID {
			if c.Kind != core.EnvConfigKindSSH {
				return nil, fmt.Errorf("配置 %s 类型 %s 不是 ssh", configID, c.Kind)
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
