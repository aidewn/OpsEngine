package store

import (
	"OpsEngine/internal/core"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// AssembleStore 集合持久化存储（TOML 文件）
type AssembleStore struct {
	baseDir string
}

// NewAssembleStore 创建集合存储实例
func NewAssembleStore(baseDir string) *AssembleStore {
	return &AssembleStore{baseDir: baseDir}
}

// List 列出所有集合
func (s *AssembleStore) List() ([]core.AssembleDef, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, err
	}

	var assembles []core.AssembleDef
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".toml") {
			a, err := s.Get(strings.TrimSuffix(entry.Name(), ".toml"))
			if err == nil {
				assembles = append(assembles, a)
			}
		}
	}
	return assembles, nil
}

// Get 按 ID 加载集合
func (s *AssembleStore) Get(id string) (core.AssembleDef, error) {
	path := filepath.Join(s.baseDir, id+".toml")
	content, err := os.ReadFile(path)
	if err != nil {
		return core.AssembleDef{}, fmt.Errorf("集合未找到: %s", id)
	}

	content = []byte(resolveEnvVars(string(content)))

	var a core.AssembleDef
	if _, err := toml.Decode(string(content), &a); err != nil {
		return core.AssembleDef{}, fmt.Errorf("TOML 解析失败: %w", err)
	}
	return a, nil
}

// Save 保存集合
func (s *AssembleStore) Save(a core.AssembleDef) error {
	path := filepath.Join(s.baseDir, a.ID+".toml")

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(a)
}

// Delete 删除集合
func (s *AssembleStore) Delete(id string) error {
	path := filepath.Join(s.baseDir, id+".toml")
	return os.Remove(path)
}
