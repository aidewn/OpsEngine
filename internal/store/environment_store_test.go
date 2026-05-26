// EnvironmentStore 单元测试
// 覆盖 CRUD 往返、空目录、非 .toml 文件忽略、不存在 ID 的错误语义

package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"OpsEngine/internal/core"
)

// 构造一个含 SSH 配置的环境样本，便于在多用例间复用
func sampleEnv() core.EnvironmentDef {
	return core.EnvironmentDef{
		ID:          "env-1",
		Name:        "nginx 生产",
		Description: "线上 nginx 集群",
		Configs: []core.EnvConfigItem{
			{
				ID:          "ssh-app",
				Name:        "应用机",
				Kind:        core.EnvConfigKindSSH,
				Description: "主 SSH 账号",
				Fields: map[string]any{
					"host":            "10.0.0.1",
					"port":            int64(22),
					"user":            "root",
					"password":        "secret",
					"timeout_seconds": int64(10),
				},
			},
			{
				ID:          "docker-app",
				Name:        "Docker over SSH",
				Kind:        core.EnvConfigKindDocker,
				Description: "复用应用机 SSH",
				Fields: map[string]any{
					"mode":          "over_ssh",
					"ssh_config_id": "ssh-app",
				},
			},
		},
	}
}

// 验证 Save → Get 完整往返，包含 Configs 顺序与 Fields 内的数字类型
func TestEnvironmentStore_SaveGetRoundTrip(t *testing.T) {
	store := NewEnvironmentStore(t.TempDir())
	env := sampleEnv()

	if err := store.Save(env); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	got, err := store.Get(env.ID)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.ID != env.ID || got.Name != env.Name || got.Description != env.Description {
		t.Fatalf("基本字段不一致: %+v", got)
	}
	if len(got.Configs) != 2 {
		t.Fatalf("Configs 数量错误: got %d", len(got.Configs))
	}
	if got.Configs[0].ID != "ssh-app" || got.Configs[1].ID != "docker-app" {
		t.Fatalf("Configs 顺序错误: %+v", got.Configs)
	}
	ssh := got.Configs[0]
	if ssh.Kind != core.EnvConfigKindSSH {
		t.Fatalf("kind 不一致: %s", ssh.Kind)
	}
	if ssh.Fields["host"] != "10.0.0.1" {
		t.Fatalf("host 不一致: %v", ssh.Fields["host"])
	}
	// TOML 整型往返：BurntSushi/toml 解码到 any 时为 int64
	if port, ok := ssh.Fields["port"].(int64); !ok || port != 22 {
		t.Fatalf("port 不一致: %T %v", ssh.Fields["port"], ssh.Fields["port"])
	}
	if to, ok := ssh.Fields["timeout_seconds"].(int64); !ok || to != 10 {
		t.Fatalf("timeout_seconds 不一致: %T %v", ssh.Fields["timeout_seconds"], ssh.Fields["timeout_seconds"])
	}
}

// 同 ID 再次 Save 应覆盖旧内容
func TestEnvironmentStore_SaveOverwrites(t *testing.T) {
	store := NewEnvironmentStore(t.TempDir())
	env := sampleEnv()
	if err := store.Save(env); err != nil {
		t.Fatalf("首次 Save 失败: %v", err)
	}

	env.Name = "改名"
	env.Configs = env.Configs[:1]
	if err := store.Save(env); err != nil {
		t.Fatalf("覆盖 Save 失败: %v", err)
	}

	got, err := store.Get(env.ID)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.Name != "改名" {
		t.Fatalf("Name 未更新: %s", got.Name)
	}
	if len(got.Configs) != 1 {
		t.Fatalf("Configs 未缩减: %d", len(got.Configs))
	}
}

// 不存在 ID 的 Get 必须返回带 "环境未找到" 提示的错误
func TestEnvironmentStore_GetNotFound(t *testing.T) {
	store := NewEnvironmentStore(t.TempDir())
	_, err := store.Get("missing")
	if err == nil {
		t.Fatal("期望返回错误，实际 nil")
	}
	if !strings.Contains(err.Error(), "环境未找到") {
		t.Fatalf("错误信息不含「环境未找到」: %v", err)
	}
}

// 空目录 List 不报错且返回空切片
func TestEnvironmentStore_ListEmptyDir(t *testing.T) {
	store := NewEnvironmentStore(t.TempDir())
	envs, err := store.List()
	if err != nil {
		t.Fatalf("空目录 List 报错: %v", err)
	}
	if len(envs) != 0 {
		t.Fatalf("期望空切片，实际 %d 条", len(envs))
	}
}

// List 必须只挑选 .toml 文件，忽略其它扩展
func TestEnvironmentStore_ListIgnoresNonToml(t *testing.T) {
	dir := t.TempDir()
	store := NewEnvironmentStore(dir)
	if err := store.Save(sampleEnv()); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}
	// 放一个干扰文件
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("ignored"), 0644); err != nil {
		t.Fatalf("写干扰文件失败: %v", err)
	}

	envs, err := store.List()
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(envs) != 1 || envs[0].ID != "env-1" {
		t.Fatalf("List 结果错误: %+v", envs)
	}
}

// Delete 不存在的 ID 返回 *PathError（与 os.Remove 一致）
func TestEnvironmentStore_DeleteNotFound(t *testing.T) {
	store := NewEnvironmentStore(t.TempDir())
	err := store.Delete("missing")
	if err == nil {
		t.Fatal("期望返回错误，实际 nil")
	}
	var pe *fs.PathError
	if !errors.As(err, &pe) {
		t.Fatalf("期望 *fs.PathError，实际 %T: %v", err, err)
	}
}
