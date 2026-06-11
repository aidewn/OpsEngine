// 节点配置 Schema 校验与环境引用校验的单元测试。
package engine

import (
	"errors"
	"strings"
	"testing"

	"OpsEngine/internal/core"
)

// stubCfgNode 是只携带 ConfigSchema 的测试节点。
type stubCfgNode struct{ def core.NodeTypeDef }

func (s stubCfgNode) TypeDef() core.NodeTypeDef          { return s.def }
func (stubCfgNode) Execute(ExecContext) (Outputs, error) { return Outputs{}, nil }

// registerStubNode 向全局注册表注册测试节点（TypeID 需唯一，进程内只注册一次）。
func registerStubNode(def core.NodeTypeDef) {
	if _, ok := Lookup(def.TypeID); !ok {
		Register(stubCfgNode{def: def})
	}
}

func i64(v int64) *int64 { return &v }

// TestValidateConfigField 覆盖 required / 类型 / select 选项 / number 范围四类规则。
func TestValidateConfigField(t *testing.T) {
	cases := []struct {
		name    string
		field   core.FieldSchema
		cfg     map[string]any
		wantErr string // 空表示应通过
	}{
		{"必填缺失", core.FieldSchema{Type: "text", ID: "cmd", Required: true}, map[string]any{}, "必填项但未提供"},
		{"必填空串", core.FieldSchema{Type: "text", ID: "cmd", Required: true}, map[string]any{"cmd": "  "}, "必填项但值为空"},
		{"必填通过", core.FieldSchema{Type: "text", ID: "cmd", Required: true}, map[string]any{"cmd": "ls"}, ""},
		{"选填缺失通过", core.FieldSchema{Type: "text", ID: "cmd"}, map[string]any{}, ""},
		{"字符串类型错", core.FieldSchema{Type: "text", ID: "cmd"}, map[string]any{"cmd": 42}, "应为字符串"},
		{"select 越界", core.FieldSchema{Type: "select", ID: "mode", Options: []string{"a", "b"}}, map[string]any{"mode": "c"}, "不在可选项"},
		{"select 合法", core.FieldSchema{Type: "select", ID: "mode", Options: []string{"a", "b"}}, map[string]any{"mode": "b"}, ""},
		{"number 类型错", core.FieldSchema{Type: "number", ID: "port"}, map[string]any{"port": "22"}, "应为数字"},
		{"number 低于下限", core.FieldSchema{Type: "number", ID: "port", Min: i64(1)}, map[string]any{"port": float64(0)}, "不能小于"},
		{"number 高于上限", core.FieldSchema{Type: "number", ID: "port", Max: i64(65535)}, map[string]any{"port": float64(70000)}, "不能大于"},
		{"number 合法", core.FieldSchema{Type: "number", ID: "port", Min: i64(1), Max: i64(65535)}, map[string]any{"port": 22}, ""},
		{"toggle 类型错", core.FieldSchema{Type: "toggle", ID: "sudo"}, map[string]any{"sudo": "yes"}, "应为布尔值"},
	}
	for _, tc := range cases {
		err := validateConfigField("n1", "test_type", tc.cfg, tc.field)
		if tc.wantErr == "" {
			if err != nil {
				t.Fatalf("%s: 不应报错，got %v", tc.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Fatalf("%s: 期望含 %q 的错误，got %v", tc.name, tc.wantErr, err)
		}
		// 错误信息必须含节点 ID 与字段 ID，AI 重试时靠它定位
		if !strings.Contains(err.Error(), "n1") || !strings.Contains(err.Error(), tc.field.ID) {
			t.Fatalf("%s: 错误信息缺节点/字段标识: %v", tc.name, err)
		}
	}
}

// TestValidateNodeConfigs 验证注册表驱动的整体校验与未注册类型跳过。
func TestValidateNodeConfigs(t *testing.T) {
	registerStubNode(core.NodeTypeDef{
		TypeID: "test_cfg_required",
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "script", Required: true},
		},
	})
	// 未注册类型（动态集合节点）应跳过
	if err := ValidateNodeConfigs([]core.NodeInstance{
		{InstanceID: "a", TypeID: "assemble:xyz", Config: map[string]any{}},
	}); err != nil {
		t.Fatalf("未注册类型应跳过，got %v", err)
	}
	// 缺必填应报错
	err := ValidateNodeConfigs([]core.NodeInstance{
		{InstanceID: "n1", TypeID: "test_cfg_required", Config: map[string]any{}},
	})
	if err == nil || !strings.Contains(err.Error(), "script") {
		t.Fatalf("应报缺必填 script，got %v", err)
	}
}

// TestValidateEnvRefs 验证环境与环境配置引用的存在性及 kind 过滤。
func TestValidateEnvRefs(t *testing.T) {
	registerStubNode(core.NodeTypeDef{
		TypeID: "test_cfg_env",
		ConfigSchema: []core.FieldSchema{
			{Type: "env_select", ID: "environment_id", Required: true},
			{Type: "env_config_select", ID: "ssh_config_id", ConfigKindFilter: "ssh"},
		},
	})
	lookup := func(id string) (core.EnvironmentDef, error) {
		if id != "env-1" {
			return core.EnvironmentDef{}, errors.New("not found")
		}
		return core.EnvironmentDef{ID: "env-1", Name: "生产", Configs: []core.EnvConfigItem{
			{ID: "ssh-1", Kind: "ssh"},
			{ID: "docker-1", Kind: "docker"},
		}}, nil
	}
	node := func(env, cfg string) []core.NodeInstance {
		return []core.NodeInstance{{
			InstanceID: "n1", TypeID: "test_cfg_env",
			Config: map[string]any{"environment_id": env, "ssh_config_id": cfg},
		}}
	}

	if err := ValidateEnvRefs(node("env-1", "ssh-1"), lookup); err != nil {
		t.Fatalf("合法引用不应报错: %v", err)
	}
	if err := ValidateEnvRefs(node("env-x", "ssh-1"), lookup); err == nil || !strings.Contains(err.Error(), "环境 env-x 不存在") {
		t.Fatalf("应报环境不存在，got %v", err)
	}
	if err := ValidateEnvRefs(node("env-1", "ssh-404"), lookup); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("应报配置不存在，got %v", err)
	}
	if err := ValidateEnvRefs(node("env-1", "docker-1"), lookup); err == nil || !strings.Contains(err.Error(), "要求 ssh") {
		t.Fatalf("应报 kind 不匹配，got %v", err)
	}
	// lookup 为 nil 时整体跳过
	if err := ValidateEnvRefs(node("env-x", "ssh-404"), nil); err != nil {
		t.Fatalf("无 lookup 应跳过: %v", err)
	}
}
