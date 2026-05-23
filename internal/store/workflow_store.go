package store

import (
	"OpsEngine/internal/core"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type WorkflowStore struct {
	baseDir string
}

func NewWorkflowStore(baseDir string) *WorkflowStore {
	return &WorkflowStore{baseDir: baseDir}
}

// List 列出所有工作流
func (s *WorkflowStore) List() ([]core.WorkflowDef, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, err
	}

	var workflows []core.WorkflowDef
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".toml") {
			wf, err := s.Get(strings.TrimSuffix(entry.Name(), ".toml"))
			if err == nil {
				workflows = append(workflows, wf)
			}
		}
	}
	return workflows, nil
}

// Get 按 ID 加载工作流
func (s *WorkflowStore) Get(id string) (core.WorkflowDef, error) {
	path := filepath.Join(s.baseDir, id+".toml")
	content, err := os.ReadFile(path)
	if err != nil {
		return core.WorkflowDef{}, fmt.Errorf("工作流未找到: %s", id)
	}

	// 处理环境变量引用
	content = []byte(resolveEnvVars(string(content)))

	var wf core.WorkflowDef
	if _, err := toml.Decode(string(content), &wf); err != nil {
		return core.WorkflowDef{}, fmt.Errorf("TOML 解析失败: %w", err)
	}
	return wf, nil
}

// Save 保存工作流（JSON 格式，方便 API 创建）
func (s *WorkflowStore) Save(wf core.WorkflowDef) error {
	// 注意：API 创建的工作流保存为 JSON，
	// 手动编写的工作流用 TOML 格式
	// 这里简化为直接用 toml 编码
	path := filepath.Join(s.baseDir, wf.ID+".toml")

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(wf)
}

// Delete 删除工作流
func (s *WorkflowStore) Delete(id string) error {
	path := filepath.Join(s.baseDir, id+".toml")
	return os.Remove(path)
}

// resolveEnvVars 替换 ${ENV:VAR_NAME} 格式的环境变量引用
func resolveEnvVars(content string) string {
	for {
		start := strings.Index(content, "${ENV:")
		if start == -1 {
			break
		}
		end := strings.Index(content[start:], "}")
		if end == -1 {
			break
		}
		placeholder := content[start : start+end+1]
		varName := content[start+6 : start+end]
		value := os.Getenv(varName)
		content = strings.Replace(content, placeholder, value, 1)
	}
	return content
}
