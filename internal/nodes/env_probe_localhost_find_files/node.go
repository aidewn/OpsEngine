// env_probe_localhost_find_files 节点
// Pure 节点（无 exec 端口）；按 resolve_mode 工作：
//   static  —— 不读盘，从 probe_snapshot 还原 output 端口值
//   dynamic —— 调用本包注册的 Probe 函数实时正则搜索本机文件
// 编辑态「探测一次」复用 Probe 函数，与 env_probe_ssh_find_files 同构
//
// 差异仅在 Probe 内部：用 filepath.WalkDir + 自带深度控制替换 SFTP Walk

package env_probe_localhost_find_files

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	"OpsEngine/internal/probe"
)

// TypeID 节点与探测函数共用同一 ID
const TypeID = "env_probe_localhost_find_files"

const (
	defaultStartDir = "/"
	defaultDepth    = 5
)

func init() {
	engine.Register(&Node{})
	probe.Register(TypeID, Probe)
}

// Node 本机文件正则搜索探测
type Node struct{}

// TypeDef 节点元信息
// 输出端口与 SSH 版本对齐：selected_path / paths / first_path / count
func (Node) TypeDef() core.NodeTypeDef {
	minDepth, maxDepth := int64(1), int64(32)
	return core.NodeTypeDef{
		TypeID:      TypeID,
		DisplayName: "环境 · 本机搜索文件",
		Category:    "environment",
		NodeKind:    core.NodeKindPure,
		Icon:        "🔎",
		Description: "在本机按正则搜索文件；编辑态可点选，运行态按 resolve_mode 还原或重探",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "selected_path", Label: "selected_path", PortType: core.PortTypeString},
			{ID: "paths", Label: "paths", PortType: core.PortTypeAny},
			{ID: "first_path", Label: "first", PortType: core.PortTypeString},
			{ID: "count", Label: "count", PortType: core.PortTypeInt},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "env_select", ID: "environment_id", Label: "环境", Required: true},
			{Type: "env_config_select", ID: "config_id", Label: "本机配置",
				Required: true, ConfigKindFilter: string(core.EnvConfigKindLocalhost)},
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

// Probe 本机正则搜索主体
// 使用 filepath.WalkDir 遍历，自行计算「相对 startDir 的深度」实现 max_depth 控制
// 遍历过程中遇到不可访问目录时跳过当前子树（fs.SkipDir）而不是整体失败
func Probe(env core.EnvironmentDef, configID string, nodeConfig map[string]any) (probe.ProbeResult, error) {
	if err := findLocalhostConfig(env, configID); err != nil {
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
	maxDepth := intField(nodeConfig, "max_depth", defaultDepth)
	if maxDepth <= 0 {
		maxDepth = defaultDepth
	}

	cleanedStart := filepath.Clean(startDir)
	startDepth := strings.Count(cleanedStart, string(filepath.Separator))

	var matches []string
	walkErr := filepath.WalkDir(cleanedStart, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// 无法访问的条目（权限/竞争）：若是目录则整棵子树跳过，否则忽略
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		// 深度控制：当前路径 - 起点深度 > maxDepth 则不再深入
		depth := strings.Count(p, string(filepath.Separator)) - startDepth
		if depth > maxDepth {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if re.MatchString(d.Name()) {
			matches = append(matches, p)
		}
		return nil
	})
	if walkErr != nil {
		return probe.ProbeResult{}, fmt.Errorf("遍历失败: %w", walkErr)
	}

	items := make([]probe.ProbeItem, 0, len(matches))
	for _, p := range matches {
		items = append(items, probe.ProbeItem{
			Key:   p,
			Label: filepath.Base(p),
		})
	}
	return probe.ProbeResult{Items: items}, nil
}

// findLocalhostConfig 校验环境中存在 localhost 配置
func findLocalhostConfig(env core.EnvironmentDef, configID string) error {
	for i := range env.Configs {
		c := &env.Configs[i]
		if c.ID == configID {
			if c.Kind != core.EnvConfigKindLocalhost {
				return fmt.Errorf("配置 %s 类型 %s 不是 localhost", configID, c.Kind)
			}
			return nil
		}
	}
	return fmt.Errorf("配置未找到: %s", configID)
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
