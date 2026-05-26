// env_probe_localhost_list_dir 单元测试
// 用 t.TempDir 造目录，覆盖 include_files / include_dirs 与 kind/path 校验

package env_probe_localhost_list_dir

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"OpsEngine/internal/core"
	"OpsEngine/internal/probe"
)

// 构造一个含 1 子目录 + 2 文件的 tmp 目录
func makeTempLayout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatalf("mkdir 失败: %v", err)
	}
	for _, name := range []string{"a.txt", "b.log"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatalf("write 失败: %v", err)
		}
	}
	return dir
}

func sampleEnv() core.EnvironmentDef {
	return core.EnvironmentDef{
		ID:      "env-1",
		Configs: []core.EnvConfigItem{{ID: "local-1", Kind: core.EnvConfigKindLocalhost}},
	}
}

// 默认 include_files=true / include_dirs=false：只列文件
func TestProbeFilesOnly(t *testing.T) {
	dir := makeTempLayout(t)
	res, err := Probe(sampleEnv(), "local-1", map[string]any{
		"path":          dir,
		"include_files": true,
		"include_dirs":  false,
	})
	if err != nil {
		t.Fatalf("Probe 失败: %v", err)
	}
	want := []string{filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.log")}
	if !equalSortedKeys(res.Items, want) {
		t.Fatalf("结果不匹配: got %v want %v", keysOf(res.Items), want)
	}
}

// include_dirs=true / include_files=false：只列子目录
func TestProbeDirsOnly(t *testing.T) {
	dir := makeTempLayout(t)
	res, err := Probe(sampleEnv(), "local-1", map[string]any{
		"path":          dir,
		"include_files": false,
		"include_dirs":  true,
	})
	if err != nil {
		t.Fatalf("Probe 失败: %v", err)
	}
	want := []string{filepath.Join(dir, "sub")}
	if !equalSortedKeys(res.Items, want) {
		t.Fatalf("结果不匹配: got %v want %v", keysOf(res.Items), want)
	}
}

// kind 错配（指向 ssh 配置）必须报错
func TestProbeRejectsNonLocalhost(t *testing.T) {
	e := core.EnvironmentDef{
		Configs: []core.EnvConfigItem{{ID: "wrong", Kind: core.EnvConfigKindSSH}},
	}
	_, err := Probe(e, "wrong", map[string]any{"path": "/tmp"})
	if err == nil {
		t.Fatal("期望报错，实际 nil")
	}
}

// 缺 path 必须报错
func TestProbeRequiresPath(t *testing.T) {
	_, err := Probe(sampleEnv(), "local-1", map[string]any{})
	if err == nil {
		t.Fatal("期望 path 错误，实际 nil")
	}
}

// ── 比较辅助 ──────────────────────────────────────────────

func keysOf(items []probe.ProbeItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Key
	}
	return out
}

func equalSortedKeys(items []probe.ProbeItem, want []string) bool {
	got := keysOf(items)
	sort.Strings(got)
	w := append([]string(nil), want...)
	sort.Strings(w)
	if len(got) != len(w) {
		return false
	}
	for i := range got {
		if got[i] != w[i] {
			return false
		}
	}
	return true
}
