// 环境资产清单（Environment Inventory）。
//
// 目的（文档 P4 / §3.1）：让 Agent 看到环境内所有配置的脱敏摘要，而不是只盯着某个 SSH。
// 这是环境级会话、架构图、多机巡检的前置数据结构。
//
// 安全红线：
//   - 密码、私钥、Token 等敏感字段一律不进入 SafeSummary。
//   - 主机地址默认脱敏（保留前 N 个字符 + 末段）。
//   - 调用方不应基于 Inventory 直接发起连接——真实连接参数永远从 environmentStore 取。

package agentcontext

import (
	"fmt"
	"sort"
	"strings"

	"OpsEngine/internal/core"
)

// Inventory 是注入 prompt 的环境资产清单。
type Inventory struct {
	EnvironmentID   string
	EnvironmentName string
	Configs         []InventoryConfig
}

// InventoryConfig 是单条配置的脱敏摘要。
type InventoryConfig struct {
	ID   string
	Name string
	Kind string
	// Summary 是脱敏后的可见字段（host/user/port/namespace 等），用于让模型理解配置形态。
	Summary map[string]string
}

// 这些字段名出现在 EnvConfigItem.Fields 中时一律不进入 prompt。
// 大小写不敏感匹配（实现里用 ToLower 后判断）。
var sensitiveFieldKeys = map[string]bool{
	"password":         true,
	"passwd":           true,
	"private_key":      true,
	"private_key_path": true,
	"key":              true,
	"token":            true,
	"secret":           true,
	"client_secret":    true,
	"access_token":     true,
	"refresh_token":    true,
	"api_token":        true,
	"api_key":          true,
	"kubeconfig":       true,
	"kubeconfig_path":  true,
	"ca_cert":          true,
	"ca_data":          true,
	"client_cert":      true,
	"client_key":       true,
}

// hostFieldKeys 命中时把值脱敏成 `***.xxx`，保留末段方便用户对照。
var hostFieldKeys = map[string]bool{
	"host":     true,
	"hostname": true,
	"server":   true,
	"endpoint": true,
	"url":      true,
}

// BuildInventory 从环境定义构造 Inventory。零拷贝原 env 数据，所有敏感字段在 sanitizeFields 中过滤。
func BuildInventory(env core.EnvironmentDef) Inventory {
	configs := make([]InventoryConfig, 0, len(env.Configs))
	for _, c := range env.Configs {
		configs = append(configs, InventoryConfig{
			ID:      c.ID,
			Name:    c.Name,
			Kind:    string(c.Kind),
			Summary: sanitizeFields(c.Fields),
		})
	}
	return Inventory{
		EnvironmentID:   env.ID,
		EnvironmentName: env.Name,
		Configs:         configs,
	}
}

// IsEmpty 报告环境是否没有任何配置，调用方据此决定是否还要注入 inventory 到 prompt。
func (inv Inventory) IsEmpty() bool { return len(inv.Configs) == 0 }

// SingleSSH 返回唯一一条 SSH 配置的 ID；若 0 条或多条则返回空串。
// inspection.PickSSHTarget 用它做"环境只有一台 SSH 时自动选定"的快路径。
func (inv Inventory) SingleSSH() string {
	var found string
	for _, c := range inv.Configs {
		if c.Kind != string(core.EnvConfigKindSSH) {
			continue
		}
		if found != "" {
			return ""
		}
		found = c.ID
	}
	return found
}

// RenderText 把 Inventory 渲染为 Markdown，注入到 LLM system 消息。
// 输出示例：
//
//	# 环境资产：生产环境
//	- SSH `web-01` (id=cfg-1): user=deploy, host=***.10.5, port=22
//	- K8s `prod-cluster` (id=cfg-3): server=https://***.example.com, namespaces=default,ops
//
// 排序：按 Kind 字典序，相同 Kind 内按 Name。
func (inv Inventory) RenderText() string {
	if inv.IsEmpty() {
		return fmt.Sprintf("# 环境资产：%s\n（该环境暂未配置任何资产）", inv.EnvironmentName)
	}
	sorted := make([]InventoryConfig, len(inv.Configs))
	copy(sorted, inv.Configs)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Kind != sorted[j].Kind {
			return sorted[i].Kind < sorted[j].Kind
		}
		return sorted[i].Name < sorted[j].Name
	})

	var sb strings.Builder
	fmt.Fprintf(&sb, "# 环境资产：%s\n", inv.EnvironmentName)
	sb.WriteString("以下是当前环境内可见的全部配置（脱敏摘要，请基于这些资产判断目标）：\n")
	for _, c := range sorted {
		fmt.Fprintf(&sb, "- %s `%s` (id=%s)", strings.ToUpper(c.Kind), c.Name, c.ID)
		if pairs := renderSummaryPairs(c.Summary); pairs != "" {
			fmt.Fprintf(&sb, ": %s", pairs)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n注意：以上摘要不含密码、私钥、Token 等敏感字段；真实连接由后端在工具执行时使用配置层数据。")
	return sb.String()
}

// renderSummaryPairs 把 SafeSummary 拼成 "k=v, k=v"，按 key 字典序，避免每次输出顺序不一致。
func renderSummaryPairs(summary map[string]string) string {
	if len(summary) == 0 {
		return ""
	}
	keys := make([]string, 0, len(summary))
	for k := range summary {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, summary[k]))
	}
	return strings.Join(parts, ", ")
}

// sanitizeFields 把 EnvConfigItem.Fields 过滤成 prompt 安全的字符串映射。
//   - 敏感字段直接丢弃
//   - 主机/URL 类字段调用 maskHost 脱敏
//   - 其他字段：原始值 → string；过长截断
func sanitizeFields(fields map[string]any) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		lk := strings.ToLower(strings.TrimSpace(k))
		if sensitiveFieldKeys[lk] {
			continue
		}
		raw := stringifyFieldValue(v)
		if raw == "" {
			continue
		}
		if hostFieldKeys[lk] {
			raw = maskHost(raw)
		}
		if len(raw) > 120 {
			raw = raw[:117] + "..."
		}
		out[lk] = raw
	}
	return out
}

// stringifyFieldValue 把 any → string，跳过 nil / 复杂结构。
// map / slice 等复杂值不进入 prompt，避免泄露未知敏感字段。
func stringifyFieldValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int, int32, int64, float32, float64:
		return fmt.Sprintf("%v", x)
	default:
		return ""
	}
}

// maskHost 把主机地址脱敏：保留首段 + 末段，中间替换为 ***。
// 例如 "10.0.123.45" → "10.***.45"；"web-01.prod.example.com" → "web-01.***.com"。
// 这样既不泄露完整地址，又让用户能对照环境列表识别。
func maskHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	// 去掉协议前缀（http:// 等）防止脱敏后变成 "https:**..com"。
	scheme := ""
	if i := strings.Index(host, "://"); i > 0 {
		scheme = host[:i+3]
		host = host[i+3:]
	}
	// 取主机部分（去掉路径和端口后缀）。
	tail := ""
	if i := strings.IndexAny(host, ":/"); i > 0 {
		tail = host[i:]
		host = host[:i]
	}
	parts := strings.Split(host, ".")
	if len(parts) <= 2 {
		// 段数不足以脱敏，直接整段打码（防止短主机名暴露）。
		return scheme + "***" + tail
	}
	masked := parts[0] + ".***." + parts[len(parts)-1]
	return scheme + masked + tail
}
