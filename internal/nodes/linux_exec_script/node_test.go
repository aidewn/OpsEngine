// linux_exec_script 节点单元测试
//
// 边界与现有 SSH 节点一致：用 fakeContext 测元数据 / 输入校验 /
// 脚本来源优先级 / 解释器选择 / fail_on_error 注入

package linux_exec_script

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
	if def.TypeID != "linux_exec_script" {
		t.Fatalf("TypeID 不匹配: %s", def.TypeID)
	}
	if def.NodeKind != core.NodeKindAction {
		t.Fatalf("NodeKind 不匹配: %s", def.NodeKind)
	}
	if def.ExecutionMode != core.ExecutionModeRemoteCmd {
		t.Fatalf("ExecutionMode 不匹配: %s", def.ExecutionMode)
	}

	wantInputs := map[string]core.PortType{
		"exec_in": core.PortTypeExec,
		"client":  core.PortTypeLinuxSsh,
		"script":  core.PortTypeString,
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
		"success":         core.PortTypeBool,
		"exit_code":       core.PortTypeInt,
		"stdout":          core.PortTypeString,
		"stderr":          core.PortTypeString,
		"combined_output": core.PortTypeString,
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
		config: map[string]any{"default_script": "echo hi"},
	})
	if err == nil || !strings.Contains(err.Error(), "缺少 client 输入") {
		t.Fatalf("期望缺少 client 错误，实际: %v", err)
	}
}

func TestExecuteRejectsWrongClientType(t *testing.T) {
	_, err := (Node{}).Execute(fakeContext{
		inputs:  map[string]any{"client": "not-client"},
		config:  map[string]any{"default_script": "echo hi"},
		handled: map[string]bool{"client": true},
	})
	if err == nil || !strings.Contains(err.Error(), "不是 LinuxSshConnection") {
		t.Fatalf("期望 client 类型错误，实际: %v", err)
	}
}

func TestExecuteRequiresScript(t *testing.T) {
	_, err := (Node{}).Execute(fakeContext{
		inputs:  map[string]any{"client": clients.NewLinuxSshClient(nil, "127.0.0.1", 22, "root")},
		handled: map[string]bool{"client": true},
	})
	if err == nil || !strings.Contains(err.Error(), "script 未配置") {
		t.Fatalf("期望 script 未配置错误，实际: %v", err)
	}
}

func TestScriptTextPrefersInput(t *testing.T) {
	got := scriptText(fakeContext{
		inputs:  map[string]any{"script": "echo from-input"},
		config:  map[string]any{"default_script": "echo from-config"},
		handled: map[string]bool{"script": true},
	})
	if got != "echo from-input" {
		t.Fatalf("应优先使用 input，实际: %q", got)
	}
}

func TestScriptTextFallsBackToConfig(t *testing.T) {
	// input 未连接（handled 未标记 script）
	got := scriptText(fakeContext{
		config: map[string]any{"default_script": "echo from-config"},
	})
	if got != "echo from-config" {
		t.Fatalf("input 未连接应 fallback config，实际: %q", got)
	}
}

func TestScriptTextEmptyInputFallsBack(t *testing.T) {
	// input 连接了但是空字符串，应 fallback config
	got := scriptText(fakeContext{
		inputs:  map[string]any{"script": ""},
		config:  map[string]any{"default_script": "echo from-config"},
		handled: map[string]bool{"script": true},
	})
	if got != "echo from-config" {
		t.Fatalf("input 为空应 fallback config，实际: %q", got)
	}
}

func TestScriptTextPreservesMultilineAndTrailingNewline(t *testing.T) {
	// 不应做 TrimSpace，保留 heredoc 等场景下有意义的格式
	in := "line1\nline2\n\n"
	got := scriptText(fakeContext{
		inputs:  map[string]any{"script": in},
		handled: map[string]bool{"script": true},
	})
	if got != in {
		t.Fatalf("script 不应被 trim，期望 %q，实际 %q", in, got)
	}
}

func TestResolveInterpreter(t *testing.T) {
	cases := []struct{ in, want string }{
		{"bash", "bash"},
		{"sh", "sh"},
		{"", "bash"},        // 空 → 默认 bash
		{"python3", "bash"}, // 未知 → 默认 bash
		{"  sh  ", "sh"},    // 含空白
	}
	for _, c := range cases {
		if got := resolveInterpreter(c.in); got != c.want {
			t.Errorf("resolveInterpreter(%q) = %q，期望 %q", c.in, got, c.want)
		}
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
func (c fakeContext) SetReturn(string, any)          {}
func (c fakeContext) Info(string, ...any)            {}
func (c fakeContext) Warn(string, ...any)            {}
func (c fakeContext) Error(string, ...any)           {}

var _ engine.ExecContext = fakeContext{}
