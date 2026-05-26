// 环境配置 TOML 存储
// 按业务/项目维度的环境定义（含 SSH/Docker/K8s/Jenkins 配置），与 workflow_store 对齐
// MVP 不引入 ${ENV:} 变量替换；凭证按 plan 文档 §13 明确明文落盘

package store

import (
	"OpsEngine/internal/core"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// EnvironmentStore 环境 TOML 文件持久化
type EnvironmentStore struct {
	baseDir string
}

// NewEnvironmentStore 创建 store 实例，baseDir 为 .toml 文件所在目录
func NewEnvironmentStore(baseDir string) *EnvironmentStore {
	return &EnvironmentStore{baseDir: baseDir}
}

// List 列出所有环境定义
// 扫描 baseDir 下所有 .toml 文件，单个解析失败跳过（与 workflow_store 一致）
func (s *EnvironmentStore) List() ([]core.EnvironmentDef, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, err
	}

	var envs []core.EnvironmentDef
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".toml") {
			env, err := s.Get(strings.TrimSuffix(entry.Name(), ".toml"))
			if err == nil {
				envs = append(envs, env)
			}
		}
	}
	return envs, nil
}

// Get 按 ID 读取环境定义
func (s *EnvironmentStore) Get(id string) (core.EnvironmentDef, error) {
	path := filepath.Join(s.baseDir, id+".toml")
	content, err := os.ReadFile(path)
	if err != nil {
		return core.EnvironmentDef{}, fmt.Errorf("环境未找到: %s", id)
	}

	var env core.EnvironmentDef
	if _, err := toml.Decode(string(content), &env); err != nil {
		return core.EnvironmentDef{}, fmt.Errorf("TOML 解析失败: %w", err)
	}
	return env, nil
}

// Save 写入 / 覆盖环境定义
func (s *EnvironmentStore) Save(env core.EnvironmentDef) error {
	path := filepath.Join(s.baseDir, env.ID+".toml")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(env)
}

// Delete 删除环境文件
func (s *EnvironmentStore) Delete(id string) error {
	path := filepath.Join(s.baseDir, id+".toml")
	return os.Remove(path)
}
