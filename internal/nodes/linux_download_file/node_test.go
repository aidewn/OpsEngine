// linux_download_file 节点单元测试
//
// 边界与现有 linux_exec_command 一致：用 fakeContext 测元数据 / 输入校验 /
// 脚本拼接 helper，不测真实 SSH 调用（SSH 是外部依赖）

package linux_download_file

import (
	"context"
	"strings"
	"testing"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func TestTypeDef(t *testing.T) {
	def := (Node{}).TypeDef()
	if def.TypeID != "linux_download_file" {
		t.Fatalf("TypeID 不匹配: %s", def.TypeID)
	}
	if def.NodeKind != core.NodeKindAction {
		t.Fatalf("NodeKind 不匹配: %s", def.NodeKind)
	}
	if def.ExecutionMode != core.ExecutionModeRemoteCmd {
		t.Fatalf("ExecutionMode 不匹配: %s", def.ExecutionMode)
	}

	wantInputs := map[string]core.PortType{
		"exec_in":   core.PortTypeExec,
		"client":    core.PortTypeLinuxSsh,
		"url":       core.PortTypeString,
		"dest_path": core.PortTypeString,
	}
	for _, p := range def.InputPorts {
		want, ok := wantInputs[p.ID]
		if !ok {
			t.Fatalf("意外输入端口: %s", p.ID)
		}
		if p.PortType != want {
			t.Fatalf("输入端口 %s 类型不匹配: %s", p.ID, p.PortType)
		}
		delete(wantInputs, p.ID)
	}
	if len(wantInputs) != 0 {
		t.Fatalf("缺少输入端口: %+v", wantInputs)
	}

	wantOutputs := map[string]core.PortType{
		"success":   core.PortTypeBool,
		"exit_code": core.PortTypeInt,
		"dest_path": core.PortTypeString,
		"tool_used": core.PortTypeString,
	}
	for _, p := range def.OutputPorts {
		want, ok := wantOutputs[p.ID]
		if !ok {
			continue
		}
		if p.PortType != want {
			t.Fatalf("输出端口 %s 类型不匹配: %s", p.ID, p.PortType)
		}
		delete(wantOutputs, p.ID)
	}
	if len(wantOutputs) != 0 {
		t.Fatalf("缺少输出端口: %+v", wantOutputs)
	}
}

func TestExecuteRequiresClient(t *testing.T) {
	_, err := (Node{}).Execute(fakeContext{
		config: map[string]any{"url": "http://x", "dest_path": "/tmp/x"},
	})
	if err == nil || !strings.Contains(err.Error(), "缺少 client 输入") {
		t.Fatalf("期望缺少 client 错误，实际: %v", err)
	}
}

func TestExecuteRejectsWrongClientType(t *testing.T) {
	_, err := (Node{}).Execute(fakeContext{
		inputs:  map[string]any{"client": "not-client"},
		config:  map[string]any{"url": "http://x", "dest_path": "/tmp/x"},
		handled: map[string]bool{"client": true},
	})
	if err == nil || !strings.Contains(err.Error(), "不是 LinuxSshConnection") {
		t.Fatalf("期望 client 类型错误，实际: %v", err)
	}
}

func TestExecuteRequiresURL(t *testing.T) {
	_, err := (Node{}).Execute(fakeContext{
		inputs:  map[string]any{"client": clients.NewLinuxSshClient(nil, "127.0.0.1", 22, "root")},
		handled: map[string]bool{"client": true},
		config:  map[string]any{"dest_path": "/tmp/x"},
	})
	if err == nil || !strings.Contains(err.Error(), "url 未配置") {
		t.Fatalf("期望 url 未配置错误，实际: %v", err)
	}
}

func TestExecuteRequiresDestPath(t *testing.T) {
	_, err := (Node{}).Execute(fakeContext{
		inputs:  map[string]any{"client": clients.NewLinuxSshClient(nil, "127.0.0.1", 22, "root")},
		handled: map[string]bool{"client": true},
		config:  map[string]any{"url": "http://x"},
	})
	if err == nil || !strings.Contains(err.Error(), "dest_path 未配置") {
		t.Fatalf("期望 dest_path 未配置错误，实际: %v", err)
	}
}

func TestShellQuote(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"abc", `'abc'`},
		{"with space", `'with space'`},
		{"O'Brien", `'O'\''Brien'`},
		{`a$b`, `'a$b'`},
		{"", `''`},
	}
	for _, c := range cases {
		if got := shellQuote(c.in); got != c.want {
			t.Errorf("shellQuote(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestBuildDownloadScript_AllOptions(t *testing.T) {
	got := buildDownloadScript(downloadOpts{
		URL:             "http://example.com/repo",
		Dest:            "/etc/yum.repos.d/example.repo",
		TimeoutSeconds:  30,
		FollowRedirects: true,
		Insecure:        true,
		EnsureParentDir: true,
	})

	mustContain := []string{
		"set -e",
		`mkdir -p "$(dirname '/etc/yum.repos.d/example.repo')"`,
		"command -v curl",
		"__OPSENGINE_TOOL__=curl",
		`curl -fsSLk --max-time 30 -o '/etc/yum.repos.d/example.repo' 'http://example.com/repo'`,
		"command -v wget",
		"__OPSENGINE_TOOL__=wget",
		`wget -q --no-check-certificate --timeout=30 -O '/etc/yum.repos.d/example.repo' 'http://example.com/repo'`,
		"exit 127",
	}
	for _, s := range mustContain {
		if !strings.Contains(got, s) {
			t.Errorf("脚本应包含 %q，实际:\n%s", s, got)
		}
	}
}

func TestBuildDownloadScript_MinimalOptions(t *testing.T) {
	got := buildDownloadScript(downloadOpts{
		URL:             "http://x",
		Dest:            "/tmp/x",
		TimeoutSeconds:  60,
		FollowRedirects: false,
		Insecure:        false,
		EnsureParentDir: false,
	})

	if strings.Contains(got, "mkdir -p") {
		t.Errorf("ensure_parent_dir=false 时脚本不应包含 mkdir -p:\n%s", got)
	}
	// curl 选项应是 -fsS（无 L、无 k）
	if !strings.Contains(got, "curl -fsS --max-time 60") {
		t.Errorf("最小选项下 curl 应为 -fsS，实际:\n%s", got)
	}
	if strings.Contains(got, "--no-check-certificate") {
		t.Errorf("insecure=false 时 wget 不应有 --no-check-certificate:\n%s", got)
	}
}

func TestParseToolUsed(t *testing.T) {
	cases := []struct{ stderr, want string }{
		{"__OPSENGINE_TOOL__=curl\n", "curl"},
		{"some warning\n__OPSENGINE_TOOL__=wget\nmore stuff\n", "wget"},
		{"no sentinel here", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := parseToolUsed(c.stderr); got != c.want {
			t.Errorf("parseToolUsed(%q) = %q，期望 %q", c.stderr, got, c.want)
		}
	}
}

func TestStripToolSentinel(t *testing.T) {
	in := "__OPSENGINE_TOOL__=curl\nreal error line\n"
	got := stripToolSentinel(in)
	if strings.Contains(got, "__OPSENGINE_TOOL__") {
		t.Errorf("应移除 sentinel 行，实际: %q", got)
	}
	if !strings.Contains(got, "real error line") {
		t.Errorf("应保留真实日志行，实际: %q", got)
	}
}

// ── fakeContext ──────────────────────────────────────────

type fakeContext struct {
	inputs  map[string]any
	config  map[string]any
	handled map[string]bool
}

func (c fakeContext) Context() context.Context { return context.Background() }
func (c fakeContext) NodeID() string           { return "node" }
func (c fakeContext) Input(portID string) (any, bool) {
	if c.handled != nil && !c.handled[portID] {
		return nil, false
	}
	v, ok := c.inputs[portID]
	return v, ok
}
func (c fakeContext) Config(fieldID string) any {
	return c.config[fieldID]
}
func (c fakeContext) ConfigString(fieldID string) string {
	v, _ := c.Config(fieldID).(string)
	return v
}
func (c fakeContext) ConfigInt(fieldID string) int64 {
	switch v := c.Config(fieldID).(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	}
	return 0
}
func (c fakeContext) ConfigBool(fieldID string) bool {
	v, _ := c.Config(fieldID).(bool)
	return v
}
func (c fakeContext) GetVariable(string) (any, bool) { return nil, false }
func (c fakeContext) SetVariable(string, any)        {}
func (c fakeContext) GetParam(string) (any, bool)    { return nil, false }
func (c fakeContext) Info(string, ...any)            {}
func (c fakeContext) Warn(string, ...any)            {}
func (c fakeContext) Error(string, ...any)           {}

var _ engine.ExecContext = fakeContext{}
