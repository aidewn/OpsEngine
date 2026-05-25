package linux_exec_command

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

	if def.TypeID != "linux_exec_command" {
		t.Fatalf("TypeID 不匹配: %s", def.TypeID)
	}
	if def.NodeKind != core.NodeKindAction {
		t.Fatalf("NodeKind 不匹配: %s", def.NodeKind)
	}
	if def.ExecutionMode != core.ExecutionModeRemoteCmd {
		t.Fatalf("ExecutionMode 不匹配: %s", def.ExecutionMode)
	}
	if len(def.InputPorts) != 3 {
		t.Fatalf("输入端口数量不匹配: %d", len(def.InputPorts))
	}
	if def.InputPorts[1].ID != "client" || def.InputPorts[1].PortType != core.PortTypeLinuxSsh {
		t.Fatalf("client 输入端口定义不匹配: %+v", def.InputPorts[1])
	}
	if len(def.OutputPorts) != 6 {
		t.Fatalf("输出端口数量不匹配: %d", len(def.OutputPorts))
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
			t.Fatalf("端口 %s 类型不匹配: %s", p.ID, p.PortType)
		}
		delete(wantOutputs, p.ID)
	}
	if len(wantOutputs) != 0 {
		t.Fatalf("缺少输出端口: %+v", wantOutputs)
	}
}

func TestExecuteRequiresClient(t *testing.T) {
	_, err := (Node{}).Execute(fakeContext{
		config: map[string]any{"command": "uname -a"},
	})
	if err == nil || !strings.Contains(err.Error(), "缺少 client 输入") {
		t.Fatalf("期望缺少 client 错误，实际: %v", err)
	}
}

func TestExecuteRejectsWrongClientType(t *testing.T) {
	_, err := (Node{}).Execute(fakeContext{
		inputs:  map[string]any{"client": "not-client"},
		config:  map[string]any{"command": "uname -a"},
		handled: map[string]bool{"client": true},
	})
	if err == nil || !strings.Contains(err.Error(), "不是 LinuxSshConnection") {
		t.Fatalf("期望 client 类型错误，实际: %v", err)
	}
}

func TestExecuteRequiresCommand(t *testing.T) {
	_, err := (Node{}).Execute(fakeContext{
		inputs:  map[string]any{"client": clients.NewLinuxSshClient(nil, "127.0.0.1", 22, "root")},
		handled: map[string]bool{"client": true},
	})
	if err == nil || !strings.Contains(err.Error(), "command 未配置") {
		t.Fatalf("期望 command 未配置错误，实际: %v", err)
	}
}

func TestCommandTextPrefersInput(t *testing.T) {
	got := commandText(fakeContext{
		inputs:  map[string]any{"command": "  whoami  "},
		config:  map[string]any{"command": "uname -a"},
		handled: map[string]bool{"command": true},
	})
	if got != "whoami" {
		t.Fatalf("命令应优先来自输入端口，实际: %q", got)
	}
}

func TestFailOnErrorDefaultTrue(t *testing.T) {
	if !failOnError(fakeContext{config: map[string]any{}}) {
		t.Fatal("未配置 fail_on_error 时应默认为 true")
	}
}

func TestFailOnErrorToggle(t *testing.T) {
	if failOnError(fakeContext{config: map[string]any{"fail_on_error": false}}) {
		t.Fatal("fail_on_error=false 时应不中断")
	}
	if !failOnError(fakeContext{config: map[string]any{"fail_on_error": true}}) {
		t.Fatal("fail_on_error=true 时应中断")
	}
}

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
