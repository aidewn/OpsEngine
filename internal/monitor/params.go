// 采集任务参数读取助手：参数经 Wails JSON 反序列化后数字多为 float64，需抹平类型。

package monitor

import "strings"

// paramString 读字符串参数，缺失或类型不符返回空串。
func paramString(p map[string]any, key string) string {
	if p == nil {
		return ""
	}
	if v, ok := p[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// paramInt 读整数参数，兼容 float64/int/int64，缺失返回 fallback。
func paramInt(p map[string]any, key string, fallback int) int {
	if p == nil {
		return fallback
	}
	v, ok := p[key]
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

// paramBool 读布尔参数，缺失返回 fallback。
func paramBool(p map[string]any, key string, fallback bool) bool {
	if p == nil {
		return fallback
	}
	if v, ok := p[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return fallback
}
