// 执行记录持久化存储（JSON 文件）
// 设计：
//   - 只持久化终态（Success/Failed/Terminated）
//   - 每条记录一个文件：data/executions/<id>.json
//   - 列表加载时全量扫描目录（MVP 阶段记录量小，无需索引）

package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"OpsEngine/internal/core"
)

// ExecutionStore 执行记录持久化
type ExecutionStore struct {
	baseDir string
	mu      sync.Mutex // 保护并发写
}

// NewExecutionStore 创建执行记录存储
func NewExecutionStore(baseDir string) *ExecutionStore {
	return &ExecutionStore{baseDir: baseDir}
}

// Save 保存一条执行记录（JSON 格式）
func (s *ExecutionStore) Save(rec core.ExecutionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.baseDir, rec.ID+".json")
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化执行记录失败: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// Get 按 ID 加载执行记录
func (s *ExecutionStore) Get(id string) (core.ExecutionRecord, error) {
	path := filepath.Join(s.baseDir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return core.ExecutionRecord{}, fmt.Errorf("执行记录未找到: %s", id)
	}
	var rec core.ExecutionRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return core.ExecutionRecord{}, fmt.Errorf("JSON 解析失败: %w", err)
	}
	return rec, nil
}

// List 列出所有已持久化的执行记录
func (s *ExecutionStore) List() ([]core.ExecutionRecord, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, err
	}
	var records []core.ExecutionRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		rec, err := s.Get(id)
		if err != nil {
			// 单条加载失败不影响其余（可能是文件损坏）
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// Delete 删除执行记录文件
// 文件不存在不报错（已被删过 / 从未持久化均视为成功）
func (s *ExecutionStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.baseDir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
