// Inventory 测试：脱敏正确性、唯一 SSH 识别、渲染稳定排序。

package agentcontext

import (
	"strings"
	"testing"

	"OpsEngine/internal/core"
)

func sampleEnv() core.EnvironmentDef {
	return core.EnvironmentDef{
		ID: "env-prod", Name: "生产环境",
		Configs: []core.EnvConfigItem{
			{ID: "cfg-ssh-1", Name: "web-01", Kind: core.EnvConfigKindSSH, Fields: map[string]any{
				"host": "10.0.123.45", "user": "deploy", "port": 22,
				"password": "should-not-leak",
			}},
			{ID: "cfg-ssh-2", Name: "db-01", Kind: core.EnvConfigKindSSH, Fields: map[string]any{
				"host": "db-01.prod.example.com", "user": "root",
				"private_key": "-----BEGIN-----...",
			}},
			{ID: "cfg-k8s", Name: "prod-cluster", Kind: core.EnvConfigKindK8s, Fields: map[string]any{
				"server": "https://k8s-api.prod.example.com:6443",
				"token":  "eyJhbG...",
			}},
		},
	}
}

// TestBuildInventoryDropsSensitive 验证密码/私钥/Token 不进入 Summary。
func TestBuildInventoryDropsSensitive(t *testing.T) {
	inv := BuildInventory(sampleEnv())
	for _, c := range inv.Configs {
		for k, v := range c.Summary {
			if sensitiveFieldKeys[k] {
				t.Fatalf("配置 %s 暴露了敏感字段 %s=%s", c.ID, k, v)
			}
			if strings.Contains(strings.ToLower(v), "should-not-leak") ||
				strings.Contains(v, "BEGIN-----") ||
				strings.HasPrefix(v, "eyJhbG") {
				t.Fatalf("配置 %s 的字段 %s 泄露了敏感原值: %s", c.ID, k, v)
			}
		}
	}
}

// TestBuildInventoryMasksHost 验证 IP 与域名都被脱敏。
func TestBuildInventoryMasksHost(t *testing.T) {
	inv := BuildInventory(sampleEnv())
	for _, c := range inv.Configs {
		for _, key := range []string{"host", "server"} {
			if v, ok := c.Summary[key]; ok {
				if !strings.Contains(v, "***") {
					t.Fatalf("%s.%s 未脱敏: %s", c.ID, key, v)
				}
			}
		}
	}
}

// TestSingleSSH 验证环境中只有一台 SSH 时能识别，多台时返回空。
func TestSingleSSH(t *testing.T) {
	env := sampleEnv()
	inv := BuildInventory(env)
	if got := inv.SingleSSH(); got != "" {
		t.Fatalf("多 SSH 应返回空，got %q", got)
	}
	// 只留一条 SSH。
	env.Configs = []core.EnvConfigItem{env.Configs[0], env.Configs[2]}
	inv = BuildInventory(env)
	if got := inv.SingleSSH(); got != "cfg-ssh-1" {
		t.Fatalf("唯一 SSH 应被识别, got %q", got)
	}
}

// TestRenderText 验证渲染产物包含环境名、配置 id、按 kind 字典序排列。
func TestRenderText(t *testing.T) {
	out := BuildInventory(sampleEnv()).RenderText()
	for _, want := range []string{"生产环境", "cfg-ssh-1", "cfg-k8s", "K8S", "SSH"} {
		if !strings.Contains(out, want) {
			t.Fatalf("渲染缺少 %q\n%s", want, out)
		}
	}
	// k8s 字典序在 ssh 之前。
	k8sIdx := strings.Index(out, "K8S")
	sshIdx := strings.Index(out, "SSH")
	if k8sIdx < 0 || sshIdx < 0 || k8sIdx >= sshIdx {
		t.Fatalf("kind 排序异常: k8s=%d ssh=%d", k8sIdx, sshIdx)
	}
}

// TestRenderTextEmpty 验证零配置环境给出明确提示。
func TestRenderTextEmpty(t *testing.T) {
	out := Inventory{EnvironmentName: "空环境"}.RenderText()
	if !strings.Contains(out, "暂未配置任何资产") {
		t.Fatalf("空环境渲染未给提示: %s", out)
	}
}

// TestMaskHostKeepsFirstAndLast 验证脱敏保留首末段。
func TestMaskHostKeepsFirstAndLast(t *testing.T) {
	cases := map[string]string{
		"10.0.123.45":             "10.***.45",
		"web-01.prod.example.com": "web-01.***.com",
		"shorthost":               "***",
		"https://api.example.com:6443/x": "https://api.***.com:6443/x",
	}
	for in, want := range cases {
		if got := maskHost(in); got != want {
			t.Fatalf("maskHost(%q) = %q, want %q", in, got, want)
		}
	}
}
