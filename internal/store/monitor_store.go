// 监控分组与监控项 TOML 存储（Phase 1：静态管理）
//
// 设计取舍：
//   - 与 environment_store / ops_doc_store 一致，单个实体一个 .toml 文件；
//   - 分组与监控项分目录存放（groups/ 与 panels/），List 时在内存按环境/分组过滤；
//   - 正常采集数据不落盘（plan §5.1），故本 store 只管理分组与监控项两类静态定义，
//     PanelState / Incident / MonitorReport 的持久化留到 Phase 3/4。

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

// MonitorStore 管理 data/monitor/ 下的分组与监控项定义。
type MonitorStore struct {
	baseDir string
	mu      sync.RWMutex
}

// NewMonitorStore 构造存储；调用方需保证 baseDir 在使用前已 mkdir。
func NewMonitorStore(baseDir string) *MonitorStore {
	return &MonitorStore{baseDir: baseDir}
}

// groupsDir / panelsDir / statesDir / incidentsDir / reportsDir 返回各类实体的子目录路径。
func (s *MonitorStore) groupsDir() string    { return filepath.Join(s.baseDir, "groups") }
func (s *MonitorStore) panelsDir() string    { return filepath.Join(s.baseDir, "panels") }
func (s *MonitorStore) statesDir() string    { return filepath.Join(s.baseDir, "states") }
func (s *MonitorStore) incidentsDir() string { return filepath.Join(s.baseDir, "incidents") }
func (s *MonitorStore) reportsDir() string   { return filepath.Join(s.baseDir, "reports") }
func (s *MonitorStore) configsDir() string   { return filepath.Join(s.baseDir, "configs") }

// defaultMonitorIntervalSeconds 是环境未配置时的默认采集间隔。
const defaultMonitorIntervalSeconds = 60

// ── 分组 ────────────────────────────────────────────────

// ListGroups 列出指定环境下的所有分组，按 Order 升序、其次按 Name。
func (s *MonitorStore) ListGroups(environmentID string) ([]core.MonitorGroup, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// 初始化为空切片而非 nil：Wails 会把 nil 切片序列化成 JSON null，
	// 前端按数组访问（如 .filter/.map）会直接抛错。
	groups := []core.MonitorGroup{}
	err := readTOMLDir(s.groupsDir(), func(id string) error {
		var g core.MonitorGroup
		if err := decodeTOMLFile(filepath.Join(s.groupsDir(), id+".toml"), &g); err != nil {
			return err
		}
		if g.EnvironmentID == environmentID {
			groups = append(groups, g)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Order != groups[j].Order {
			return groups[i].Order < groups[j].Order
		}
		return groups[i].Name < groups[j].Name
	})
	return groups, nil
}

// GetGroup 按 ID 读取分组。
func (s *MonitorStore) GetGroup(id string) (core.MonitorGroup, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var g core.MonitorGroup
	if err := decodeTOMLFile(filepath.Join(s.groupsDir(), id+".toml"), &g); err != nil {
		return core.MonitorGroup{}, fmt.Errorf("监控分组未找到: %s", id)
	}
	return g, nil
}

// SaveGroup 整体覆盖保存分组。
func (s *MonitorStore) SaveGroup(g core.MonitorGroup) error {
	if strings.TrimSpace(g.ID) == "" {
		return fmt.Errorf("分组 ID 不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return encodeTOMLFile(s.groupsDir(), g.ID, g)
}

// DeleteGroup 删除分组。注意：调用方需自行处理分组下监控项的归属。
func (s *MonitorStore) DeleteGroup(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return removeTOMLFile(s.groupsDir(), id)
}

// ── 监控项 ──────────────────────────────────────────────

// ListPanels 列出指定环境（可选指定分组）下的监控项，按 Name 升序。
// groupID 为空串时返回该环境下全部监控项。
func (s *MonitorStore) ListPanels(environmentID, groupID string) ([]core.MonitorPanel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// 同 ListGroups：空切片而非 nil，避免前端拿到 null 后崩溃。
	panels := []core.MonitorPanel{}
	err := readTOMLDir(s.panelsDir(), func(id string) error {
		var p core.MonitorPanel
		if err := decodeTOMLFile(filepath.Join(s.panelsDir(), id+".toml"), &p); err != nil {
			return err
		}
		if p.EnvironmentID != environmentID {
			return nil
		}
		if groupID != "" && p.GroupID != groupID {
			return nil
		}
		panels = append(panels, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(panels, func(i, j int) bool { return panels[i].Name < panels[j].Name })
	return panels, nil
}

// GetPanel 按 ID 读取监控项。
func (s *MonitorStore) GetPanel(id string) (core.MonitorPanel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var p core.MonitorPanel
	if err := decodeTOMLFile(filepath.Join(s.panelsDir(), id+".toml"), &p); err != nil {
		return core.MonitorPanel{}, fmt.Errorf("监控项未找到: %s", id)
	}
	return p, nil
}

// SavePanel 整体覆盖保存监控项。
func (s *MonitorStore) SavePanel(p core.MonitorPanel) error {
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("监控项 ID 不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return encodeTOMLFile(s.panelsDir(), p.ID, p)
}

// DeletePanel 删除监控项。
func (s *MonitorStore) DeletePanel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return removeTOMLFile(s.panelsDir(), id)
}

// ── 监控项状态（PanelState）─────────────────────────────
//
// 只持久化状态（plan §5.2），不保存正常采集数据；状态按 panelID 一文件。

// GetPanelState 按 panelID 读取状态；不存在时返回 (zero, false, nil)。
func (s *MonitorStore) GetPanelState(panelID string) (core.PanelState, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var st core.PanelState
	err := decodeTOMLFile(filepath.Join(s.statesDir(), panelID+".toml"), &st)
	if err != nil {
		if os.IsNotExist(err) {
			return core.PanelState{}, false, nil
		}
		return core.PanelState{}, false, err
	}
	return st, true, nil
}

// SavePanelState 覆盖保存监控项状态。
func (s *MonitorStore) SavePanelState(st core.PanelState) error {
	if strings.TrimSpace(st.PanelID) == "" {
		return fmt.Errorf("PanelState.PanelID 不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return encodeTOMLFile(s.statesDir(), st.PanelID, st)
}

// DeletePanelState 删除监控项状态（监控项删除时调用，避免残留）。
func (s *MonitorStore) DeletePanelState(panelID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return removeTOMLFile(s.statesDir(), panelID)
}

// ── 异常事件（Incident）─────────────────────────────────

// GetIncident 按 ID 读取异常事件。
func (s *MonitorStore) GetIncident(id string) (core.Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var inc core.Incident
	if err := decodeTOMLFile(filepath.Join(s.incidentsDir(), id+".toml"), &inc); err != nil {
		return core.Incident{}, fmt.Errorf("异常事件未找到: %s", id)
	}
	return inc, nil
}

// SaveIncident 覆盖保存异常事件。
func (s *MonitorStore) SaveIncident(inc core.Incident) error {
	if strings.TrimSpace(inc.ID) == "" {
		return fmt.Errorf("Incident.ID 不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return encodeTOMLFile(s.incidentsDir(), inc.ID, inc)
}

// ListIncidents 列出指定环境的异常事件，按开始时间倒序（最新在前）。
func (s *MonitorStore) ListIncidents(environmentID string) ([]core.Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	incidents := []core.Incident{}
	err := readTOMLDir(s.incidentsDir(), func(id string) error {
		var inc core.Incident
		if err := decodeTOMLFile(filepath.Join(s.incidentsDir(), id+".toml"), &inc); err != nil {
			return err
		}
		if inc.EnvironmentID == environmentID {
			incidents = append(incidents, inc)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(incidents, func(i, j int) bool { return incidents[i].StartedAt.After(incidents[j].StartedAt) })
	return incidents, nil
}

// ── 诊断报告（MonitorReport）─────────────────────────────

// GetReport 按 ID 读取诊断报告。
func (s *MonitorStore) GetReport(id string) (core.MonitorReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rep core.MonitorReport
	if err := decodeTOMLFile(filepath.Join(s.reportsDir(), id+".toml"), &rep); err != nil {
		return core.MonitorReport{}, fmt.Errorf("诊断报告未找到: %s", id)
	}
	return rep, nil
}

// SaveReport 覆盖保存诊断报告。
func (s *MonitorStore) SaveReport(rep core.MonitorReport) error {
	if strings.TrimSpace(rep.ID) == "" {
		return fmt.Errorf("MonitorReport.ID 不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return encodeTOMLFile(s.reportsDir(), rep.ID, rep)
}

// ListReports 列出指定环境的诊断报告，按创建时间倒序。
func (s *MonitorStore) ListReports(environmentID string) ([]core.MonitorReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	reports := []core.MonitorReport{}
	err := readTOMLDir(s.reportsDir(), func(id string) error {
		var rep core.MonitorReport
		if err := decodeTOMLFile(filepath.Join(s.reportsDir(), id+".toml"), &rep); err != nil {
			return err
		}
		if rep.EnvironmentID == environmentID {
			reports = append(reports, rep)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].CreatedAt.After(reports[j].CreatedAt) })
	return reports, nil
}

// ── 监控调度配置（MonitorConfig）────────────────────────

// GetConfig 读取环境监控配置；不存在时返回默认（未开启、默认间隔）。
func (s *MonitorStore) GetConfig(environmentID string) (core.MonitorConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var c core.MonitorConfig
	err := decodeTOMLFile(filepath.Join(s.configsDir(), environmentID+".toml"), &c)
	if err != nil {
		if os.IsNotExist(err) {
			return core.MonitorConfig{
				EnvironmentID:   environmentID,
				Enabled:         false,
				IntervalSeconds: defaultMonitorIntervalSeconds,
			}, nil
		}
		return core.MonitorConfig{}, err
	}
	return c, nil
}

// SaveConfig 覆盖保存环境监控配置。
func (s *MonitorStore) SaveConfig(c core.MonitorConfig) error {
	if strings.TrimSpace(c.EnvironmentID) == "" {
		return fmt.Errorf("MonitorConfig.EnvironmentID 不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return encodeTOMLFile(s.configsDir(), c.EnvironmentID, c)
}

// ListConfigs 列出所有已保存的环境监控配置（供调度器选取启用项）。
func (s *MonitorStore) ListConfigs() ([]core.MonitorConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	configs := []core.MonitorConfig{}
	err := readTOMLDir(s.configsDir(), func(id string) error {
		var c core.MonitorConfig
		if err := decodeTOMLFile(filepath.Join(s.configsDir(), id+".toml"), &c); err != nil {
			return err
		}
		configs = append(configs, c)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return configs, nil
}

// ── 内部 TOML 文件助手 ──────────────────────────────────

// readTOMLDir 遍历目录下所有 .toml 文件，对每个 id 调用 fn。
// 目录不存在视为空集（与 ops_doc_store 一致）；单个文件解析失败由 fn 决定是否跳过。
func readTOMLDir(dir string, fn func(id string) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".toml")
		// 单条解析失败跳过，避免一个坏文件拖垮整张列表
		if err := fn(id); err != nil {
			continue
		}
	}
	return nil
}

// decodeTOMLFile 读取并解析单个 TOML 文件到 v。
func decodeTOMLFile(path string, v any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = toml.Decode(string(content), v)
	return err
}

// encodeTOMLFile 将 v 编码写入 dir/id.toml，自动创建目录。
func encodeTOMLFile(dir, id string, v any) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	f, err := os.Create(filepath.Join(dir, id+".toml"))
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(v)
}

// removeTOMLFile 删除 dir/id.toml，文件不存在不视为错误。
func removeTOMLFile(dir, id string) error {
	if err := os.Remove(filepath.Join(dir, id+".toml")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
