// PickSSHTarget 测试：覆盖三条决策路径。

package inspection

import (
	"strings"
	"testing"

	"OpsEngine/internal/core"
)

func envWithConfigs(items ...core.EnvConfigItem) core.EnvironmentDef {
	return core.EnvironmentDef{ID: "env-1", Configs: items}
}

// TestPickSSHTargetExplicit 验证显式指定时按 id 命中。
func TestPickSSHTargetExplicit(t *testing.T) {
	env := envWithConfigs(
		core.EnvConfigItem{ID: "a", Kind: core.EnvConfigKindSSH},
		core.EnvConfigItem{ID: "b", Kind: core.EnvConfigKindSSH},
	)
	got, err := PickSSHTarget(env, "b")
	if err != nil || got != "b" {
		t.Fatalf("got %q err %v", got, err)
	}
}

// TestPickSSHTargetExplicitWrongKind 验证显式指定到非 SSH 配置时报错。
func TestPickSSHTargetExplicitWrongKind(t *testing.T) {
	env := envWithConfigs(core.EnvConfigItem{ID: "a", Kind: core.EnvConfigKindDocker})
	if _, err := PickSSHTarget(env, "a"); err == nil || !strings.Contains(err.Error(), "不是 SSH") {
		t.Fatalf("expected kind mismatch, got %v", err)
	}
}

// TestPickSSHTargetSingleAuto 验证只有一台 SSH 时自动选定。
func TestPickSSHTargetSingleAuto(t *testing.T) {
	env := envWithConfigs(
		core.EnvConfigItem{ID: "ssh-only", Kind: core.EnvConfigKindSSH},
		core.EnvConfigItem{ID: "docker", Kind: core.EnvConfigKindDocker},
	)
	got, err := PickSSHTarget(env, "")
	if err != nil || got != "ssh-only" {
		t.Fatalf("got %q err %v", got, err)
	}
}

// TestPickSSHTargetMultiAmbiguous 验证多台 SSH 且未指定时报错追问。
func TestPickSSHTargetMultiAmbiguous(t *testing.T) {
	env := envWithConfigs(
		core.EnvConfigItem{ID: "a", Kind: core.EnvConfigKindSSH},
		core.EnvConfigItem{ID: "b", Kind: core.EnvConfigKindSSH},
	)
	if _, err := PickSSHTarget(env, ""); err == nil || !strings.Contains(err.Error(), "请明确指定") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

// TestPickSSHTargetNoSSH 验证零 SSH 时报错。
func TestPickSSHTargetNoSSH(t *testing.T) {
	env := envWithConfigs(core.EnvConfigItem{ID: "docker", Kind: core.EnvConfigKindDocker})
	if _, err := PickSSHTarget(env, ""); err == nil || !strings.Contains(err.Error(), "没有 SSH") {
		t.Fatalf("expected no-SSH error, got %v", err)
	}
}
