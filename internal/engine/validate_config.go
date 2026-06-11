// 节点配置 Schema 校验与环境引用存在性校验。
// 目标：把"运行时才爆"的配置错误拦在落盘前；错误信息必须含节点 ID + 字段 ID，
// AI 生成路径靠这句话在重试循环中自动定位修正。
//
// 调用策略（见 docs/agent-workflow-integration-plan.md P2）：
//   - AI 生成/更新路径（workflow.Materialize*）：硬错误
//   - 人工保存路径（app.UpdateWorkflow）：暂以日志警告过渡，避免存量带病数据卡住保存

package engine

import (
	"fmt"
	"strings"

	"OpsEngine/internal/core"
)

// stringFieldTypes 是值为字符串的 FieldSchema.Type 集合（除 number/toggle/select 外全部）。
var stringFieldTypes = map[string]bool{
	"text": true, "password": true, "textarea": true,
	"variable_select": true, "param_select": true, "return_select": true,
	"env_select": true, "env_config_select": true,
}

// ValidateNodeConfigs 按注册表中的 ConfigSchema 逐节点校验 config：
// required 字段非空、类型匹配、select 值在 options 内、number 范围。
// 未注册类型（assemble:* 等动态节点）跳过——它们没有静态 schema。
func ValidateNodeConfigs(nodes []core.NodeInstance) error {
	for _, n := range nodes {
		node, ok := Lookup(n.TypeID)
		if !ok {
			continue
		}
		for _, f := range node.TypeDef().ConfigSchema {
			if err := validateConfigField(n.InstanceID, n.TypeID, n.Config, f); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateConfigField 校验单个配置字段。错误信息格式固定：节点 X（type）的配置 Y …
func validateConfigField(instanceID, typeID string, cfg map[string]any, f core.FieldSchema) error {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("节点 %s（%s）的配置 %s "+format,
			append([]any{instanceID, typeID, f.ID}, args...)...)
	}
	val, present := cfg[f.ID]
	if !present || val == nil {
		if f.Required {
			return fail("为必填项但未提供")
		}
		return nil
	}

	switch {
	case f.Type == "number":
		num, ok := toFloat(val)
		if !ok {
			return fail("应为数字，实际为 %T", val)
		}
		if f.Min != nil && num < float64(*f.Min) {
			return fail("不能小于 %d", *f.Min)
		}
		if f.Max != nil && num > float64(*f.Max) {
			return fail("不能大于 %d", *f.Max)
		}
	case f.Type == "toggle":
		if _, ok := val.(bool); !ok {
			return fail("应为布尔值，实际为 %T", val)
		}
	case f.Type == "select":
		s, ok := val.(string)
		if !ok {
			return fail("应为字符串，实际为 %T", val)
		}
		if f.Required && strings.TrimSpace(s) == "" {
			return fail("为必填项但值为空")
		}
		if s != "" && len(f.Options) > 0 && !containsString(f.Options, s) {
			return fail("值 %q 不在可选项 %v 内", s, f.Options)
		}
	case stringFieldTypes[f.Type]:
		s, ok := val.(string)
		if !ok {
			return fail("应为字符串，实际为 %T", val)
		}
		if f.Required && strings.TrimSpace(s) == "" {
			return fail("为必填项但值为空")
		}
	}
	return nil
}

// EnvLookup 按 ID 取环境定义，由调用方（runtime / app）注入。
type EnvLookup func(environmentID string) (core.EnvironmentDef, error)

// ValidateEnvRefs 校验节点 config 中 env_select / env_config_select 字段引用的
// 环境与环境配置真实存在；env_config_select 还会校验 ConfigKindFilter 的 kind 匹配。
// 未注册类型跳过；lookup 为 nil 时不校验（调用方未接环境存储）。
func ValidateEnvRefs(nodes []core.NodeInstance, lookup EnvLookup) error {
	if lookup == nil {
		return nil
	}
	for _, n := range nodes {
		node, ok := Lookup(n.TypeID)
		if !ok {
			continue
		}
		schema := node.TypeDef().ConfigSchema
		env, envFieldID, err := resolveNodeEnv(n, schema, lookup)
		if err != nil {
			return err
		}
		for _, f := range schema {
			if f.Type != "env_config_select" {
				continue
			}
			cfgID, _ := n.Config[f.ID].(string)
			cfgID = strings.TrimSpace(cfgID)
			if cfgID == "" {
				continue // 空值由 ValidateNodeConfigs 的 required 校验兜底
			}
			if envFieldID == "" {
				return fmt.Errorf("节点 %s（%s）的配置 %s 引用了环境配置 %s，但 schema 中没有 env_select 字段定位环境",
					n.InstanceID, n.TypeID, f.ID, cfgID)
			}
			item, found := findEnvConfig(env, cfgID)
			if !found {
				return fmt.Errorf("节点 %s（%s）的配置 %s 引用的环境配置 %s 在环境「%s」中不存在",
					n.InstanceID, n.TypeID, f.ID, cfgID, env.Name)
			}
			if f.ConfigKindFilter != "" && string(item.Kind) != f.ConfigKindFilter {
				return fmt.Errorf("节点 %s（%s）的配置 %s 引用的配置 %s 类型为 %s，要求 %s",
					n.InstanceID, n.TypeID, f.ID, cfgID, item.Kind, f.ConfigKindFilter)
			}
		}
	}
	return nil
}

// resolveNodeEnv 找到节点 schema 中的 env_select 字段并加载对应环境。
// 节点没有 env_select 字段或值为空时返回零值环境（envFieldID 标识是否存在该字段）。
func resolveNodeEnv(n core.NodeInstance, schema []core.FieldSchema, lookup EnvLookup) (core.EnvironmentDef, string, error) {
	for _, f := range schema {
		if f.Type != "env_select" {
			continue
		}
		envID, _ := n.Config[f.ID].(string)
		envID = strings.TrimSpace(envID)
		if envID == "" {
			return core.EnvironmentDef{}, f.ID, nil
		}
		env, err := lookup(envID)
		if err != nil {
			return core.EnvironmentDef{}, f.ID, fmt.Errorf("节点 %s（%s）的配置 %s 引用的环境 %s 不存在",
				n.InstanceID, n.TypeID, f.ID, envID)
		}
		return env, f.ID, nil
	}
	return core.EnvironmentDef{}, "", nil
}

// findEnvConfig 在环境内按 ID 查配置项。
func findEnvConfig(env core.EnvironmentDef, configID string) (core.EnvConfigItem, bool) {
	for _, c := range env.Configs {
		if c.ID == configID {
			return c, true
		}
	}
	return core.EnvConfigItem{}, false
}

// toFloat 把 JSON/TOML 反序列化出的数字类型统一成 float64。
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// containsString 报告 s 是否在 list 中。
func containsString(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
