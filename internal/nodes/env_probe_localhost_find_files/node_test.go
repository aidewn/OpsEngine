// env_probe_localhost_find_files 单元测试
// 用 t.TempDir 造两层文件，验证正则、case_sensitive、max_depth 行为

package env_probe_localhost_find_files

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"OpsEngine/internal/core"
	"OpsEngine/internal/probe"
)

// 目录结构（FOO.log 与 foo.log 放在不同子目录，避免在 macOS APFS 等大小写不敏感 FS 上互相覆盖）：
//
//	<tmp>/foo.log
//	<tmp>/lower/foo.log
//	<tmp>/upper/FOO.log
//	<tmp>/sub/bar.log
//	<tmp>/sub/skip.txt
func makeTempTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"sub", "lower", "upper"} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatalf("mkdir %s 失败: %v", sub, err)
		}
	}
	files := map[string]string{
		"foo.log":       "x",
		"lower/foo.log": "x",
		"upper/FOO.log": "x",
		"sub/bar.log":   "x",
		"sub/skip.txt":  "x",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s 失败: %v", name, err)
		}
	}
	return dir
}

func sampleEnv() core.EnvironmentDef {
	return core.EnvironmentDef{
		Configs: []core.EnvConfigItem{{ID: "local-1", Kind: core.EnvConfigKindLocalhost}},
	}
}

// 区分大小写：仅命中两处真正叫 foo.log 的文件，不命中 upper/FOO.log
func TestProbeCaseSensitive(t *testing.T) {
	dir := makeTempTree(t)
	res, err := Probe(sampleEnv(), "local-1", map[string]any{
		"pattern":        `^foo\.log$`,
		"start_dir":      dir,
		"max_depth":      int64(5),
		"case_sensitive": true,
	})
	if err != nil {
		t.Fatalf("Probe 失败: %v", err)
	}
	want := []string{
		filepath.Join(dir, "foo.log"),
		filepath.Join(dir, "lower", "foo.log"),
	}
	if !equalSortedKeys(res.Items, want) {
		t.Fatalf("结果不匹配: got %v want %v", keysOf(res.Items), want)
	}
}

// 不区分大小写：还应额外命中 upper/FOO.log
func TestProbeCaseInsensitive(t *testing.T) {
	dir := makeTempTree(t)
	res, err := Probe(sampleEnv(), "local-1", map[string]any{
		"pattern":        `^foo\.log$`,
		"start_dir":      dir,
		"max_depth":      int64(5),
		"case_sensitive": false,
	})
	if err != nil {
		t.Fatalf("Probe 失败: %v", err)
	}
	want := []string{
		filepath.Join(dir, "foo.log"),
		filepath.Join(dir, "lower", "foo.log"),
		filepath.Join(dir, "upper", "FOO.log"),
	}
	if !equalSortedKeys(res.Items, want) {
		t.Fatalf("结果不匹配: got %v want %v", keysOf(res.Items), want)
	}
}

// max_depth=1 时不应递归到 sub/ 内
func TestProbeMaxDepth(t *testing.T) {
	dir := makeTempTree(t)
	res, err := Probe(sampleEnv(), "local-1", map[string]any{
		"pattern":        `\.log$`,
		"start_dir":      dir,
		"max_depth":      int64(1),
		"case_sensitive": true,
	})
	if err != nil {
		t.Fatalf("Probe 失败: %v", err)
	}
	// depth=1 只命中根目录下 foo.log / FOO.log，不应包含 sub/bar.log
	for _, it := range res.Items {
		if filepath.Dir(it.Key) != dir {
			t.Fatalf("max_depth=1 不应命中 %s", it.Key)
		}
	}
}

// kind 错配必须报错
func TestProbeRejectsNonLocalhost(t *testing.T) {
	e := core.EnvironmentDef{
		Configs: []core.EnvConfigItem{{ID: "wrong", Kind: core.EnvConfigKindSSH}},
	}
	_, err := Probe(e, "wrong", map[string]any{"pattern": `.*`, "start_dir": "/tmp"})
	if err == nil {
		t.Fatal("期望报错，实际 nil")
	}
}

// 缺 pattern 必须报错
func TestProbeRequiresPattern(t *testing.T) {
	_, err := Probe(sampleEnv(), "local-1", map[string]any{"start_dir": "/tmp"})
	if err == nil {
		t.Fatal("期望 pattern 错误，实际 nil")
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
