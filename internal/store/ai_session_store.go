// AI 会话存储：TOML 文件 / 单文件一个会话，按 UpdatedAt 倒序列出。

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"OpsEngine/internal/core"

	"github.com/BurntSushi/toml"
)

// AISessionStore 是 data/ai-sessions/ 目录的简单封装，文件锁保证并发安全。
type AISessionStore struct {
	baseDir string
	mu      sync.RWMutex
}

// NewAISessionStore 构造存储实例，调用前请确保 baseDir 已 mkdir。
func NewAISessionStore(baseDir string) *AISessionStore {
	return &AISessionStore{baseDir: baseDir}
}

// List 返回所有会话，按 UpdatedAt 倒序（最新在前）。
func (s *AISessionStore) List() ([]core.AISession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []core.AISession{}, nil
		}
		return nil, err
	}
	sessions := make([]core.AISession, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".toml")
		session, err := s.loadLocked(id)
		if err != nil {
			continue
		}
		sessions = append(sessions, session)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

// Get 按 ID 加载会话。
func (s *AISessionStore) Get(id string) (core.AISession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadLocked(id)
}

// Save 整体覆盖保存。调用方负责设置 UpdatedAt。
func (s *AISessionStore) Save(session core.AISession) error {
	if strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("会话 ID 不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.baseDir, session.ID+".toml")
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("保存 AI 会话失败: %w", err)
	}
	defer file.Close()
	return toml.NewEncoder(file).Encode(session)
}

// Delete 删除会话文件。
func (s *AISessionStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.baseDir, id+".toml")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// loadLocked 读取单个会话文件，调用方持锁。
func (s *AISessionStore) loadLocked(id string) (core.AISession, error) {
	path := filepath.Join(s.baseDir, id+".toml")
	content, err := os.ReadFile(path)
	if err != nil {
		return core.AISession{}, fmt.Errorf("AI 会话未找到: %s", id)
	}
	var session core.AISession
	if _, err := toml.Decode(string(content), &session); err != nil {
		return core.AISession{}, fmt.Errorf("解析 AI 会话失败: %w", err)
	}
	return session, nil
}
