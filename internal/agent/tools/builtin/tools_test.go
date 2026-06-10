// 内置工具的纯逻辑测试（path 校验、参数提取、注册整合）。
// 不测试真实 SSH 连接，避免依赖运行环境。

package builtin

import (
	"strings"
	"testing"

	"OpsEngine/internal/agent/tools"
	"OpsEngine/internal/core"
)

// TestRegisterAllBuiltinTools 验证全部内置工具都能进 Registry（隐式校验 Tier=Read 且无重名）。
func TestRegisterAllBuiltinTools(t *testing.T) {
	reg := tools.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	want := []string{
		"env_inventory",
		"ssh_inspect", "ssh_list_dir", "ssh_read_log", "ssh_read_file", "ssh_find_files", "ssh_process_list",
		"docker_list_containers", "docker_container_logs", "docker_container_inspect", "docker_list_images",
		"k8s_list_pods", "k8s_list_workloads", "k8s_describe_pod",
		"jenkins_list_jobs",
		"local_list_dir", "local_find_files",
	}
	for _, n := range want {
		if _, ok := reg.Lookup(n); !ok {
			t.Fatalf("缺少工具 %s", n)
		}
	}
	if got := len(reg.List()); got != len(want) {
		t.Fatalf("工具总数不匹配：got %d, want %d", got, len(want))
	}
}

// TestPickEnvConfigDecision 验证通用 picker 的三条分支：
//  1. 偏好命中 → 用之
//  2. 唯一匹配 → 自动选定
//  3. 多匹配 → 报错并列出候选
func TestPickEnvConfigDecision(t *testing.T) {
	envWith := func(items ...core.EnvConfigItem) func(string) (core.EnvironmentDef, error) {
		return func(string) (core.EnvironmentDef, error) {
			return core.EnvironmentDef{ID: "e", Name: "env", Configs: items}, nil
		}
	}
	// 偏好命中
	ctx1 := tools.ToolContext{
		EnvironmentID: "e", PreferredConfigID: "b",
		EnvLookup: envWith(
			core.EnvConfigItem{ID: "a", Kind: core.EnvConfigKindSSH},
			core.EnvConfigItem{ID: "b", Kind: core.EnvConfigKindDocker},
		),
	}
	cfg, _, err := pickEnvConfig(ctx1, core.EnvConfigKindDocker)
	if err != nil || cfg.ID != "b" {
		t.Fatalf("偏好命中失败: %v %#v", err, cfg)
	}
	// 唯一匹配
	ctx2 := tools.ToolContext{
		EnvironmentID: "e",
		EnvLookup: envWith(
			core.EnvConfigItem{ID: "a", Kind: core.EnvConfigKindK8s},
		),
	}
	cfg, _, err = pickEnvConfig(ctx2, core.EnvConfigKindK8s)
	if err != nil || cfg.ID != "a" {
		t.Fatalf("唯一匹配失败: %v %#v", err, cfg)
	}
	// 多匹配
	ctx3 := tools.ToolContext{
		EnvironmentID: "e",
		EnvLookup: envWith(
			core.EnvConfigItem{ID: "a", Name: "alpha", Kind: core.EnvConfigKindDocker},
			core.EnvConfigItem{ID: "b", Name: "beta", Kind: core.EnvConfigKindDocker},
		),
	}
	if _, _, err := pickEnvConfig(ctx3, core.EnvConfigKindDocker); err == nil ||
		!strings.Contains(err.Error(), "alpha") ||
		!strings.Contains(err.Error(), "beta") {
		t.Fatalf("多匹配应报错并列候选: %v", err)
	}
	// 零匹配
	ctx4 := tools.ToolContext{
		EnvironmentID: "e",
		EnvLookup:     envWith(core.EnvConfigItem{ID: "a", Kind: core.EnvConfigKindSSH}),
	}
	if _, _, err := pickEnvConfig(ctx4, core.EnvConfigKindJenkins); err == nil ||
		!strings.Contains(err.Error(), "没有") {
		t.Fatalf("零匹配应报错: %v", err)
	}
}

// TestLooksLikeSafePath 覆盖正反两面用例。
func TestLooksLikeSafePath(t *testing.T) {
	good := []string{"/var/log/nginx", "/etc/hosts", "/", "/a/b-c_d.e"}
	bad := []string{"relative/path", "/etc;rm -rf /", "/var/`cat /etc/shadow`", "/x|y", "/a\n/b", "/$HOME"}
	for _, p := range good {
		if !looksLikeSafePath(p) {
			t.Fatalf("正常路径被拒绝: %q", p)
		}
	}
	for _, p := range bad {
		if looksLikeSafePath(p) {
			t.Fatalf("危险路径未拦截: %q", p)
		}
	}
}

// TestEnvInventoryExecute 验证 env_inventory 在缺 EnvLookup 时报错、正常时返回包含环境名的文本。
func TestEnvInventoryExecute(t *testing.T) {
	if _, err := (EnvInventory{}).Execute(tools.ToolContext{}, nil); err == nil {
		t.Fatal("EnvLookup 缺失应报错")
	}
	ctx := tools.ToolContext{
		EnvironmentID: "env-1",
		EnvLookup: func(id string) (core.EnvironmentDef, error) {
			return core.EnvironmentDef{ID: id, Name: "测试环境"}, nil
		},
	}
	res, err := (EnvInventory{}).Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.DisplaySummary == "" || res.Output == "" {
		t.Fatalf("结果不应为空: %#v", res)
	}
}

// TestArgIntCoercion 验证 JSON 数字（float64）能被正确转成 int。
func TestArgIntCoercion(t *testing.T) {
	if got := argInt(map[string]any{"n": float64(42)}, "n", 0); got != 42 {
		t.Fatalf("float64 转 int 失败: %d", got)
	}
	if got := argInt(map[string]any{}, "n", 7); got != 7 {
		t.Fatalf("缺失时未走默认: %d", got)
	}
	if got := argInt(map[string]any{"n": "bad"}, "n", 9); got != 9 {
		t.Fatalf("非数字类型应回退默认: %d", got)
	}
}
