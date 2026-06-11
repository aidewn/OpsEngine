// 版本快照机制单元测试：快照、轮替、恢复。
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"OpsEngine/internal/core"
)

// TestSnapshotAndRestore 验证保存触发快照、列表倒序与恢复链路。
func TestSnapshotAndRestore(t *testing.T) {
	dir := t.TempDir()
	s := NewWorkflowStore(dir)

	wf := core.WorkflowDef{ID: "wf-1", Name: "v1"}
	if err := s.Save(wf); err != nil {
		t.Fatalf("首次保存: %v", err)
	}
	// 首次保存无旧文件，不应产生快照
	if versions, _ := s.ListVersions("wf-1"); len(versions) != 0 {
		t.Fatalf("首次保存不应有快照: %v", versions)
	}

	wf.Name = "v2"
	time.Sleep(5 * time.Millisecond) // 保证时间戳文件名不同
	if err := s.Save(wf); err != nil {
		t.Fatalf("二次保存: %v", err)
	}
	versions, err := s.ListVersions("wf-1")
	if err != nil || len(versions) != 1 {
		t.Fatalf("应有 1 份快照: %v %v", versions, err)
	}

	// 恢复 v1：当前文件应变回 v1，且恢复前的 v2 也被快照
	restored, err := s.RestoreVersion("wf-1", versions[0].Version)
	if err != nil || restored.Name != "v1" {
		t.Fatalf("恢复失败: %#v %v", restored, err)
	}
	current, err := s.Get("wf-1")
	if err != nil || current.Name != "v1" {
		t.Fatalf("当前版本应为 v1: %#v %v", current, err)
	}
	if versions, _ := s.ListVersions("wf-1"); len(versions) != 2 {
		t.Fatalf("恢复操作本身应产生快照（共 2 份）: %v", versions)
	}
}

// TestHistoryPrune 验证超出 20 份时删最旧。
func TestHistoryPrune(t *testing.T) {
	dir := t.TempDir()
	hd := historyDir(dir, "wf-1")
	if err := os.MkdirAll(hd, 0o755); err != nil {
		t.Fatal(err)
	}
	// 直接铺 25 个递增时间戳文件
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	for i := 0; i < 25; i++ {
		name := base.Add(time.Duration(i)*time.Second).Format(versionTimeLayout) + ".toml"
		if err := os.WriteFile(filepath.Join(hd, name), []byte(fmt.Sprintf("name = \"v%d\"", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneHistory(hd); err != nil {
		t.Fatalf("prune: %v", err)
	}
	names, _ := listVersionFiles(hd)
	if len(names) != maxHistoryVersions {
		t.Fatalf("应保留 %d 份，实际 %d", maxHistoryVersions, len(names))
	}
	// 保留的应是最新 20 份（v5..v24），最旧的 v0 文件已删
	oldest := base.Format(versionTimeLayout)
	if names[0] == oldest {
		t.Fatalf("最旧版本应被删除: %v", names[0])
	}
}

// TestReadVersionRejectsTraversal 验证非法版本号被拒绝。
func TestReadVersionRejectsTraversal(t *testing.T) {
	if _, err := readVersion(t.TempDir(), "wf-1", "../../etc/passwd"); err == nil {
		t.Fatal("路径穿越应被拒绝")
	}
}
