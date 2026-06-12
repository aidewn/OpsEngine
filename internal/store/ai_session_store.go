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
	session, err := s.loadLocked(id)
	s.mu.RUnlock()
	// 解析失败的会话文件已损坏：隔离它（改名 .corrupt），避免每次加载都反复报错。
	// 隔离用独立写锁，与上面的读锁分离，避免锁内改文件的竞态。
	if err != nil && strings.Contains(err.Error(), "解析 AI 会话失败") {
		s.quarantineCorrupt(id)
	}
	return session, err
}

// quarantineCorrupt 把损坏的会话文件改名为 .corrupt（保留以备查），best-effort。
func (s *AISessionStore) quarantineCorrupt(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := filepath.Join(s.baseDir, id+".toml")
	if _, err := os.Stat(src); err != nil {
		return // 已被处理或不存在
	}
	_ = os.Rename(src, src+".corrupt")
}

// Save 整体覆盖保存。调用方负责设置 UpdatedAt。
func (s *AISessionStore) Save(session core.AISession) error {
	if strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("会话 ID 不能为空")
	}
	// 兜底：清洗所有文本字段为合法 UTF-8，杜绝任何上游截断产生的半字符把 TOML 写坏，
	// 导致整个会话文件下次加载失败（曾因进度文本按字节截断中文触发）。
	sanitizeSessionUTF8(&session)
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

// sanitizeSessionUTF8 把会话消息中可能含非法 UTF-8 的文本字段就地替换为合法字符串。
// 非法字节用替换符替换（strings.ToValidUTF8），既保证可写入又便于事后发现。
func sanitizeSessionUTF8(session *core.AISession) {
	clean := func(s string) string { return strings.ToValidUTF8(s, "�") }
	for i := range session.Messages {
		m := &session.Messages[i]
		m.Content = clean(m.Content)
		m.ChangeSummary = clean(m.ChangeSummary)
		for j := range m.Progress {
			m.Progress[j] = clean(m.Progress[j])
		}
		for j := range m.Views {
			m.Views[j].Title = clean(m.Views[j].Title)
			m.Views[j].Data = clean(m.Views[j].Data)
		}
	}
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
	// 兼容老会话：Scope 字段加入前的持久化数据不带 Scope，按 ConfigID 是否填充推断。
	if session.Scope == "" {
		if strings.TrimSpace(session.ConfigID) != "" {
			session.Scope = core.AISessionScopeConfig
		} else if strings.TrimSpace(session.EnvironmentID) == "" {
			session.Scope = core.AISessionScopeGeneral
		} else {
			session.Scope = core.AISessionScopeEnvironment
		}
	}
	return session, nil
}
