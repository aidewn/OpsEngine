package ssh_with_linux

import (
	"context"
	"strings"
	"testing"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func TestTypeDef(t *testing.T) {
	def := (Node{}).TypeDef()

	if def.TypeID != "ssh_with_linux" {
		t.Fatalf("TypeID 不匹配: %s", def.TypeID)
	}
	if def.NodeKind != core.NodeKindAction {
		t.Fatalf("NodeKind 不匹配: %s", def.NodeKind)
	}
	if def.ExecutionMode != core.ExecutionModeRemoteCmd {
		t.Fatalf("ExecutionMode 不匹配: %s", def.ExecutionMode)
	}
	if len(def.OutputPorts) != 2 {
		t.Fatalf("输出端口数量不匹配: %d", len(def.OutputPorts))
	}
	if def.OutputPorts[1].ID != "client" || def.OutputPorts[1].PortType != core.PortTypeLinuxSsh {
		t.Fatalf("client 输出端口定义不匹配: %+v", def.OutputPorts[1])
	}
}

func TestExecuteRequiresHost(t *testing.T) {
	_, err := (Node{}).Execute(fakeContext{
		config: map[string]any{
			"user":     "root",
			"password": "secret",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "host 未配置") {
		t.Fatalf("期望 host 未配置错误，实际: %v", err)
	}
}

func TestExecuteRequiresPassword(t *testing.T) {
	_, err := (Node{}).Execute(fakeContext{
		config: map[string]any{
			"host": "127.0.0.1",
			"user": "root",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "password 未配置") {
		t.Fatalf("期望 password 未配置错误，实际: %v", err)
	}
}

type fakeContext struct {
	config map[string]any
}

func (c fakeContext) Context() context.Context { return context.Background() }
func (c fakeContext) NodeID() string           { return "node" }
func (c fakeContext) Input(string) (any, bool) { return nil, false }
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
