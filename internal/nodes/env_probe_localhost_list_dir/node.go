// env_probe_localhost_list_dir 节点
// Pure 节点（无 exec 端口）；按 resolve_mode 工作：
//   static  —— 不读磁盘，从 probe_snapshot 还原 output 端口值
//   dynamic —— 调用本包注册的 Probe 函数实时读本机目录
// 编辑态「探测一次」复用 Probe 函数（由 internal/probe registry 暴露给 Wails 层）
//
// 与 env_probe_ssh_list_dir 形态对齐，差异仅在 Probe 内部用 os.ReadDir 替代 SFTP

package env_probe_localhost_list_dir

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	"OpsEngine/internal/probe"
)

// TypeID 节点与探测函数共用同一 ID
const TypeID = "env_probe_localhost_list_dir"

func init() {
	engine.Register(&Node{})
	probe.Register(TypeID, Probe)
}

// Node Pure 节点：本机列目录探测结果作为数据源
type Node struct{}

// TypeDef 节点元信息
// 输出端口与 SSH 版本一致：selected_path / paths / count
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      TypeID,
		DisplayName: "环境 · 本机列目录",
		Category:    "environment",
		NodeKind:    core.NodeKindPure,
		Icon:        "📁",
		Description: "列本机目录条目；编辑态可探测点选，运行态按 resolve_mode 还原或重读",
		InputPorts:  []core.PortDef{},
		OutputPorts: []core.PortDef{
			{ID: "selected_path", Label: "selected_path", PortType: core.PortTypeString},
			{ID: "paths", Label: "paths", PortType: core.PortTypeAny},
			{ID: "count", Label: "count", PortType: core.PortTypeInt},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "env_select", ID: "environment_id", Label: "环境", Required: true},
			{Type: "env_config_select", ID: "config_id", Label: "本机配置",
				Required: true, ConfigKindFilter: string(core.EnvConfigKindLocalhost)},
			{Type: "text", ID: "path", Label: "目录路径", Required: true,
				Placeholder: "/tmp"},
			{Type: "toggle", ID: "include_files", Label: "包含文件", Default: true},
			{Type: "toggle", ID: "include_dirs", Label: "包含子目录", Default: false},
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

// executeStatic 从 probe_snapshot 反序列化 output 端口值
func executeStatic(ctx engine.ExecContext) (engine.Outputs, error) {
	raw := ctx.Config("probe_snapshot")
	snap, _ := raw.(map[string]any)
	if snap == nil {
		return engine.Outputs{
			"selected_path": "",
			"paths":         []string{},
			"count":         int64(0),
		}, nil
	}
	picked, _ := snap["picked_key"].(string)
	items := snapshotItems(snap["items"])
	paths := make([]string, 0, len(items))
	for _, it := range items {
		paths = append(paths, it.Key)
	}
	return engine.Outputs{
		"selected_path": picked,
		"paths":         paths,
		"count":         int64(len(paths)),
	}, nil
}

// executeDynamic 复用 Probe 函数实时探测；selected_path 沿用 snapshot 兜底首项
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
		"path":          ctx.ConfigString("path"),
		"include_files": ctx.ConfigBool("include_files"),
		"include_dirs":  ctx.ConfigBool("include_dirs"),
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
	if picked == "" && len(paths) > 0 {
		picked = paths[0]
	}

	return engine.Outputs{
		"selected_path": picked,
		"paths":         paths,
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

// Probe 本机列目录主体（编辑态 / dynamic 运行态共用）
// localhost 配置 fields 为空，仅做存在 + kind 校验
func Probe(env core.EnvironmentDef, configID string, nodeConfig map[string]any) (probe.ProbeResult, error) {
	if err := findLocalhostConfig(env, configID); err != nil {
		return probe.ProbeResult{}, err
	}
	target := strings.TrimSpace(stringField(nodeConfig, "path"))
	if target == "" {
		return probe.ProbeResult{}, fmt.Errorf("path 未配置")
	}
	includeFiles := boolField(nodeConfig, "include_files", true)
	includeDirs := boolField(nodeConfig, "include_dirs", false)

	entries, err := os.ReadDir(target)
	if err != nil {
		return probe.ProbeResult{}, fmt.Errorf("列目录失败: %w", err)
	}

	items := make([]probe.ProbeItem, 0, len(entries))
	for _, entry := range entries {
		isDir := entry.IsDir()
		if isDir && !includeDirs {
			continue
		}
		if !isDir && !includeFiles {
			continue
		}
		full := filepath.Join(target, entry.Name())
		kind := "file"
		if isDir {
			kind = "dir"
		}
		meta := map[string]any{
			"type": kind,
		}
		// FileInfo 偶尔会失败（权限/竞争）；失败时跳过 size/mode 元数据但保留条目
		if info, err := entry.Info(); err == nil {
			meta["size_bytes"] = info.Size()
			meta["mode"] = info.Mode().String()
		}
		items = append(items, probe.ProbeItem{
			Key:   full,
			Label: entry.Name(),
			Meta:  meta,
		})
	}
	return probe.ProbeResult{Items: items}, nil
}

// findLocalhostConfig 校验环境中存在 localhost 配置（fields 不读，仅断言 kind）
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
