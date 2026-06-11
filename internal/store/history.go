// 资产版本快照：保存前把旧文件拷入 history 目录，支持列出与恢复。
// 目录结构：{baseDir}/history/{assetID}/{时间戳}.toml，每个资产保留最近 maxHistoryVersions 份。
// WorkflowStore 与 AssembleStore 共用本文件的辅助函数。

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// maxHistoryVersions 每个资产保留的历史版本数，超出删最旧。
const maxHistoryVersions = 20

// versionTimeLayout 历史文件名中的时间戳格式（Windows 文件名不允许冒号）。
const versionTimeLayout = "20060102T150405.000"

// VersionInfo 是一个历史版本的元信息。
type VersionInfo struct {
	// Version 即历史文件名中的时间戳，作为恢复时的句柄。
	Version string `json:"version"`
	// SavedAt 该版本被快照的时间。
	SavedAt time.Time `json:"saved_at"`
}

// historyDir 返回资产的历史目录路径。
func historyDir(baseDir, id string) string {
	return filepath.Join(baseDir, "history", id)
}

// snapshotBeforeSave 把当前文件快照进 history 并裁剪旧版本。
// 当前文件不存在（首次保存）时为 no-op。
func snapshotBeforeSave(baseDir, id string) error {
	current := filepath.Join(baseDir, id+".toml")
	data, err := os.ReadFile(current)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	dir := historyDir(baseDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := time.Now().Format(versionTimeLayout) + ".toml"
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return err
	}
	return pruneHistory(dir)
}

// pruneHistory 删除超出 maxHistoryVersions 的最旧版本。
func pruneHistory(dir string) error {
	names, err := listVersionFiles(dir)
	if err != nil {
		return err
	}
	for len(names) > maxHistoryVersions {
		if err := os.Remove(filepath.Join(dir, names[0]+".toml")); err != nil {
			return err
		}
		names = names[1:]
	}
	return nil
}

// listVersionFiles 返回历史目录下的版本名（去后缀），按时间升序。
func listVersionFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".toml") {
			names = append(names, strings.TrimSuffix(e.Name(), ".toml"))
		}
	}
	sort.Strings(names) // 时间戳格式字典序即时间序
	return names, nil
}

// listVersions 把版本文件名转成 VersionInfo，按时间倒序（最新在前）。
func listVersions(baseDir, id string) ([]VersionInfo, error) {
	names, err := listVersionFiles(historyDir(baseDir, id))
	if err != nil {
		return nil, err
	}
	out := make([]VersionInfo, 0, len(names))
	for i := len(names) - 1; i >= 0; i-- {
		t, err := time.ParseInLocation(versionTimeLayout, names[i], time.Local)
		if err != nil {
			continue // 非法文件名跳过
		}
		out = append(out, VersionInfo{Version: names[i], SavedAt: t})
	}
	return out, nil
}

// readVersion 读出指定历史版本的原始 TOML 内容。
// version 必须是合法时间戳格式，顺带防住路径穿越。
func readVersion(baseDir, id, version string) ([]byte, error) {
	if !isVersionName(version) {
		return nil, fmt.Errorf("非法版本号: %s", version)
	}
	data, err := os.ReadFile(filepath.Join(historyDir(baseDir, id), version+".toml"))
	if err != nil {
		return nil, fmt.Errorf("历史版本 %s 不存在", version)
	}
	return data, nil
}

// isVersionName 校验版本名是否符合时间戳格式。
func isVersionName(v string) bool {
	_, err := time.ParseInLocation(versionTimeLayout, v, time.Local)
	return err == nil
}
