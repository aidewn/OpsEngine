// 通用辅助：按 EnvConfigKind 选定环境内的目标配置。
//
// 决策顺序（与 inspection.PickSSHTarget 一致）：
//  1. PreferredConfigID 命中目标 kind → 用之
//  2. 环境内有且仅有一条该 kind 配置 → 自动选定
//  3. 多条 → 报错（v1 不做交互追问，让模型把"先列环境"作为前置工具）
//  4. 零条 → 报错
//
// 注意：这里不复用 inspection.SSHTargetSelectionError，因为非 SSH 场景的 target_select
// 事件协议还没接入；多 kind 模糊时返回普通 error，让 chat 工具循环把错误回传给模型，
// 模型可以提示用户"环境内有多个 X 配置，请明确指定"。

package builtin

import (
	"fmt"
	"strings"

	"OpsEngine/internal/agent/tools"
	"OpsEngine/internal/core"
)

// pickEnvConfig 选出 ctx 所属环境内、指定 kind 的目标配置。
// 返回完整 EnvConfigItem（含 Fields），调用方可直接用于拨号或调 probe。
func pickEnvConfig(ctx tools.ToolContext, kind core.EnvConfigKind) (core.EnvConfigItem, core.EnvironmentDef, error) {
	if ctx.EnvLookup == nil {
		return core.EnvConfigItem{}, core.EnvironmentDef{}, fmt.Errorf("环境查询未注入")
	}
	env, err := ctx.EnvLookup(ctx.EnvironmentID)
	if err != nil {
		return core.EnvConfigItem{}, core.EnvironmentDef{}, err
	}
	preferred := strings.TrimSpace(ctx.PreferredConfigID)
	if preferred != "" {
		for _, c := range env.Configs {
			if c.ID != preferred {
				continue
			}
			if c.Kind != kind {
				// 偏好配置 kind 不匹配，走"自动选唯一"分支兜底
				break
			}
			return c, env, nil
		}
	}
	matches := make([]core.EnvConfigItem, 0)
	for _, c := range env.Configs {
		if c.Kind == kind {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 0:
		return core.EnvConfigItem{}, env, fmt.Errorf("环境 %s 内没有 %s 配置", env.ID, kind)
	case 1:
		return matches[0], env, nil
	default:
		// 多条：列出候选 ID 让模型/用户决定。返回 error，下次让用户明确指定。
		ids := make([]string, 0, len(matches))
		for _, m := range matches {
			ids = append(ids, fmt.Sprintf("%s(%s)", m.Name, m.ID))
		}
		return core.EnvConfigItem{}, env, fmt.Errorf(
			"环境 %s 内有 %d 条 %s 配置，请用户明确指定要使用的：%s",
			env.ID, len(matches), kind, strings.Join(ids, " / "),
		)
	}
}

// argBool 是从 LLM args 提取布尔的便捷函数；缺失时返回 dflt。
func argBool(args map[string]any, key string, dflt bool) bool {
	v, ok := args[key]
	if !ok {
		return dflt
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(x, "true")
	default:
		return dflt
	}
}

// argStringOptional 从 args 取可选字符串（缺失返回 ""）。
func argStringOptional(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

// listView 把已序列化的列表 JSON 包装成视图载荷，供 docker/k8s/jenkins 等列表类工具复用。
// dataJSON 直接复用工具已构造的 entries JSON——不重复采集、不重复序列化。
func listView(kind, title, dataJSON string) *core.AIViewPayload {
	return &core.AIViewPayload{Kind: kind, Title: title, Data: dataJSON}
}
