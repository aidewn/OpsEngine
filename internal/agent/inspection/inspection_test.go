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

// TestMaterializeRejectsDangerous 验证危险命令被拦截（仅 Kind=shell 走黑名单）。
func TestMaterializeRejectsDangerous(t *testing.T) {
	plan := Plan{
		Items: []Item{
			{Title: "清理", Command: "rm -rf /tmp/old"},
		},
	}
	_, err := Materialize(plan, "env-1", "ssh-1")
	if err == nil || !strings.Contains(err.Error(), "危险") {
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

// TestRenderCommandEachKind 覆盖每种 ItemKind 的命令生成。
// 同时检查参数注入防护：路径含 `;` 应被拒绝。
func TestRenderCommandEachKind(t *testing.T) {
	cases := []struct {
		name      string
		item      Item
		want      string
		wantError bool
	}{
		{"shell 原样", Item{Kind: ItemKindShell, Command: "uname -a"}, "uname -a", false},
		{"shell 危险拒绝", Item{Kind: ItemKindShell, Command: "rm -rf /"}, "", true},
		{"read_file", Item{Kind: ItemKindReadFile, Path: "/etc/nginx/nginx.conf"},
			"head -c 65536 '/etc/nginx/nginx.conf' 2>&1", false},
		{"read_file 拒绝相对路径", Item{Kind: ItemKindReadFile, Path: "etc/passwd"}, "", true},
		{"read_file 拒绝注入", Item{Kind: ItemKindReadFile, Path: "/etc/passwd;cat /etc/shadow"}, "", true},
		{"read_log 默认 tail", Item{Kind: ItemKindReadLog, Path: "/var/log/nginx/error.log"},
			"tail -n 100 '/var/log/nginx/error.log' 2>&1", false},
		{"read_log 指定 tail", Item{Kind: ItemKindReadLog, Path: "/var/log/app.log", TailLines: 50},
			"tail -n 50 '/var/log/app.log' 2>&1", false},
		{"find_files 默认 *", Item{Kind: ItemKindFindFiles, Root: "/etc/nginx"},
			"find '/etc/nginx' -maxdepth 5 -name '*' 2>/dev/null | head -n 200", false},
		{"find_files 指定 pattern", Item{Kind: ItemKindFindFiles, Root: "/etc", Pattern: "*.conf"},
			"find '/etc' -maxdepth 5 -name '*.conf' 2>/dev/null | head -n 200", false},
		{"find_files 拒绝注入 pattern", Item{Kind: ItemKindFindFiles, Root: "/etc", Pattern: "*;rm"}, "", true},
		{"docker_list", Item{Kind: ItemKindDockerList},
			"docker ps -a --format '{{.Names}}\\t{{.Image}}\\t{{.Status}}' 2>&1", false},
		{"docker_logs", Item{Kind: ItemKindDockerLogs, Container: "nginx", TailLines: 300},
			"docker logs --tail 300 'nginx' 2>&1", false},
		{"docker_logs 默认 tail", Item{Kind: ItemKindDockerLogs, Container: "nginx"},
			"docker logs --tail 200 'nginx' 2>&1", false},
		{"k8s_pods 无参", Item{Kind: ItemKindK8sPods}, "kubectl get pods -o wide 2>&1", false},
		{"k8s_pods 带 ns + label", Item{Kind: ItemKindK8sPods, Namespace: "prod", LabelSelector: "app=web"},
			"kubectl get pods -n 'prod' -l 'app=web' -o wide 2>&1", false},
		{"k8s_describe", Item{Kind: ItemKindK8sDescribe, Workload: "Deployment/nginx", Namespace: "prod"},
			"kubectl describe 'Deployment/nginx' -n 'prod' 2>&1", false},
		{"systemd", Item{Kind: ItemKindSystemd, Service: "nginx"},
			"systemctl status 'nginx' --no-pager 2>&1", false},
		{"port_listen", Item{Kind: ItemKindPortListen},
			"(ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null) | head -n 80", false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderCommand(tt.item)
			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error, got command %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("RenderCommand error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q\nwant %q", got, tt.want)
			}
		})
	}
}

// TestPlanValidatePerKind 验证每种 Kind 必填字段缺失会被 Validate 拒绝。
func TestPlanValidatePerKind(t *testing.T) {
	cases := []struct {
		name string
		item Item
	}{
		{"shell 缺 command", Item{Title: "x", Kind: ItemKindShell}},
		{"read_file 缺 path", Item{Title: "x", Kind: ItemKindReadFile}},
		{"read_log 缺 path", Item{Title: "x", Kind: ItemKindReadLog}},
		{"find_files 缺 root", Item{Title: "x", Kind: ItemKindFindFiles}},
		{"docker_logs 缺 container", Item{Title: "x", Kind: ItemKindDockerLogs}},
		{"k8s_describe 缺 workload", Item{Title: "x", Kind: ItemKindK8sDescribe}},
		{"k8s_describe workload 非 kind/name 形式", Item{Title: "x", Kind: ItemKindK8sDescribe, Workload: "nginx"}},
		{"systemd 缺 service", Item{Title: "x", Kind: ItemKindSystemd}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			plan := Plan{Items: []Item{tt.item}}
			if err := plan.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// TestMaterializeMixedKinds 端到端：多种 Kind 混合的 Plan 能跑通 engine.ValidateWorkflow。
func TestMaterializeMixedKinds(t *testing.T) {
	plan := Plan{
		Name: "混合巡检",
		Items: []Item{
			{Title: "系统", Kind: ItemKindShell, Command: "uname -a"},
			{Title: "nginx.conf", Kind: ItemKindReadFile, Path: "/etc/nginx/nginx.conf"},
			{Title: "error.log", Kind: ItemKindReadLog, Path: "/var/log/nginx/error.log", TailLines: 50},
			{Title: "容器", Kind: ItemKindDockerList},
			{Title: "K8s pods", Kind: ItemKindK8sPods, Namespace: "default"},
			{Title: "端口", Kind: ItemKindPortListen},
		},
	}
	wf, err := Materialize(plan, "env-1", "ssh-1")
	if err != nil {
		t.Fatalf("Materialize error: %v", err)
	}
	// 节点：system_ready + env_connect_ssh + 6 个 linux_exec_command = 8
	if len(wf.Nodes) != 8 {
		t.Fatalf("nodes = %d, want 8", len(wf.Nodes))
	}
	// 检查每个 linux_exec_command 的 config.kind 与 Plan 对齐
	wantKinds := []string{"shell", "read_file", "read_log", "docker_list", "k8s_pods", "port_listen"}
	gotKinds := []string{}
	for _, n := range wf.Nodes {
		if n.TypeID != "linux_exec_command" {
			continue
		}
		k, _ := n.Config["kind"].(string)
		gotKinds = append(gotKinds, k)
	}
	if strings.Join(gotKinds, ",") != strings.Join(wantKinds, ",") {
		t.Fatalf("kinds = %v, want %v", gotKinds, wantKinds)
	}
}
