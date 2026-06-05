// OpsDoc 文档存储：sidecar 模式（<id>.toml + <id>.md）。
//
// 设计决策：
//   - 元数据走 TOML，与项目其余存储一致；
//   - Markdown 正文走独立 .md 文件，避免 TOML 多行转义、方便用户外部编辑/查看；
//   - 列表只读取 .toml，避免大文档拖慢 List；
//   - 删除时两个文件一起删，单一失败不致命（用 os.Remove + IsNotExist 兜底）。

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

// OpsDocStore 管理 data/docs/ 下的文档资产。
type OpsDocStore struct {
	baseDir string
	mu      sync.RWMutex
}

// NewOpsDocStore 构造存储；调用方需保证 baseDir 在使用前已 mkdir。
func NewOpsDocStore(baseDir string) *OpsDocStore {
	return &OpsDocStore{baseDir: baseDir}
}

// Save 整体覆盖保存：元数据 + 正文。调用方负责设置 UpdatedAt。
func (s *OpsDocStore) Save(doc core.OpsDoc) error {
	if strings.TrimSpace(doc.ID) == "" {
		return fmt.Errorf("文档 ID 不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.baseDir, 0755); err != nil {
		return fmt.Errorf("创建文档目录失败: %w", err)
	}
	tomlPath := filepath.Join(s.baseDir, doc.ID+".toml")
	mdPath := filepath.Join(s.baseDir, doc.ID+".md")

	file, err := os.Create(tomlPath)
	if err != nil {
		return fmt.Errorf("保存文档元数据失败: %w", err)
	}
	if err := toml.NewEncoder(file).Encode(doc); err != nil {
		file.Close()
		return fmt.Errorf("编码文档元数据失败: %w", err)
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.WriteFile(mdPath, []byte(doc.Body), 0644); err != nil {
		return fmt.Errorf("保存文档正文失败: %w", err)
	}
	return nil
}

// Get 按 ID 加载完整文档（元数据 + 正文）。
func (s *OpsDocStore) Get(id string) (core.OpsDoc, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadLocked(id)
}

// List 列出所有文档摘要，按 UpdatedAt 倒序。
// 只读 .toml，不加载 Body，避免大文档拖慢列表。
func (s *OpsDocStore) List() ([]core.OpsDocSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []core.OpsDocSummary{}, nil
		}
		return nil, err
	}
	out := make([]core.OpsDocSummary, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".toml")
		doc, err := s.loadMetaLocked(id)
		if err != nil {
			continue
		}
		out = append(out, doc.Summary())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// Delete 删除文档（元数据 + 正文）。两个文件任一存在都会被清掉。
func (s *OpsDocStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, path := range []string{
		filepath.Join(s.baseDir, id+".toml"),
		filepath.Join(s.baseDir, id+".md"),
	} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// loadLocked 同时读元数据和正文。调用方持锁。
func (s *OpsDocStore) loadLocked(id string) (core.OpsDoc, error) {
	doc, err := s.loadMetaLocked(id)
	if err != nil {
		return core.OpsDoc{}, err
	}
	mdPath := filepath.Join(s.baseDir, id+".md")
	body, err := os.ReadFile(mdPath)
	if err != nil && !os.IsNotExist(err) {
		return core.OpsDoc{}, fmt.Errorf("读取文档正文失败: %w", err)
	}
	doc.Body = string(body)
	return doc, nil
}

// loadMetaLocked 只读元数据，调用方持锁。
func (s *OpsDocStore) loadMetaLocked(id string) (core.OpsDoc, error) {
	path := filepath.Join(s.baseDir, id+".toml")
	content, err := os.ReadFile(path)
	if err != nil {
		return core.OpsDoc{}, fmt.Errorf("文档未找到: %s", id)
	}
	var doc core.OpsDoc
	if _, err := toml.Decode(string(content), &doc); err != nil {
		return core.OpsDoc{}, fmt.Errorf("解析文档元数据失败: %w", err)
	}
	return doc, nil
}
