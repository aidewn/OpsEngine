// 巡检计划解析与 materialize 测试。

package inspection

import (
	"strings"
	"testing"

	_ "OpsEngine/internal/nodes" // 触发节点注册，让 engine.ValidateWorkflow 在测试中可用
)

// TestParsePlan 验证从带 Markdown 围栏的回复中提取计划。
func TestParsePlan(t *testing.T) {
	reply := "```json\n" + `{
  "name": "Linux 基础巡检",
  "description": "采集 CPU/内存/磁盘",
  "items": [
    {"title": "系统信息", "command": "uname -a"},
    {"title": "磁盘", "command": "df -h"}
  ]
}` + "\n```"
	plan, err := ParsePlan(reply)
	if err != nil {
		t.Fatalf("ParsePlan error: %v", err)
	}
	if plan.Name != "Linux 基础巡检" || len(plan.Items) != 2 {
		t.Fatalf("plan unexpected: %#v", plan)
	}
}

// TestParsePlanEmptyItems 验证空 items 被拒绝。
func TestParsePlanEmptyItems(t *testing.T) {
	if _, err := ParsePlan(`{"name":"x","items":[]}`); err == nil {
		t.Fatal("expected error for empty items")
	}
}

// TestMaterialize 验证生成的工作流结构正确并通过 engine 校验。
func TestMaterialize(t *testing.T) {
	plan := Plan{
		Name: "测试巡检",
		Items: []Item{
			{Title: "系统信息", Command: "uname -a"},
			{Title: "磁盘", Command: "df -h"},
		},
	}
	wf, err := Materialize(plan, "env-1", "ssh-1")
	if err != nil {
		t.Fatalf("Materialize error: %v", err)
	}
	// 节点：system_ready + env_connect_ssh + 2 个 linux_exec_command = 4
	if len(wf.Nodes) != 4 {
		t.Fatalf("nodes = %d, want 4: %#v", len(wf.Nodes), wf.Nodes)
	}
	// 边：1 (ready→ssh) + 2 项 × 2 边（exec 串 + SSH 扇出）= 5
	if len(wf.Edges) != 5 {
		t.Fatalf("edges = %d, want 5", len(wf.Edges))
	}
	// 检查 env_connect_ssh 的 config 写入了用户的环境/配置 id。
	var sshConfig map[string]any
	for _, n := range wf.Nodes {
		if n.TypeID == "env_connect_ssh" {
			sshConfig = n.Config
			break
		}
	}
	if sshConfig["environment_id"] != "env-1" || sshConfig["config_id"] != "ssh-1" {
		t.Fatalf("SSH 节点 config 未正确填入: %#v", sshConfig)
	}
}

// TestMaterializeRejectsDangerous 验证危险命令被拦截。
func TestMaterializeRejectsDangerous(t *testing.T) {
	plan := Plan{
		Items: []Item{
			{Title: "清理", Command: "rm -rf /tmp/old"},
		},
	}
	_, err := Materialize(plan, "env-1", "ssh-1")
	if err == nil || !strings.Contains(err.Error(), "危险命令") {
		t.Fatalf("expected dangerous-command error, got %v", err)
	}
}

// TestMaterializeRequiresEnvironment 验证未绑定环境时拒绝。
func TestMaterializeRequiresEnvironment(t *testing.T) {
	plan := Plan{Items: []Item{{Title: "x", Command: "uname"}}}
	if _, err := Materialize(plan, "", "ssh-1"); err == nil {
		t.Fatal("expected error when environmentID is empty")
	}
}
