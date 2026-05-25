package linux_open_file

import (
	"context"
	"strings"
	"testing"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func TestTypeDef(t *testing.T) {
	def := (Node{}).TypeDef()
	if def.TypeID != "linux_open_file" {
		t.Fatalf("TypeID 不匹配: %s", def.TypeID)
	}
	want := map[string]core.PortType{
		"handle":     core.PortTypeLinuxFileHandle,
		"exists":     core.PortTypeBool,
		"size_bytes": core.PortTypeInt,
	}
	for _, p := range def.OutputPorts {
		if exp, ok := want[p.ID]; ok && p.PortType != exp {
			t.Fatalf("端口 %s 类型不匹配", p.ID)
		}
	}
}

func TestExecuteRequiresClient(t *testing.T) {
	_, err := (Node{}).Execute(fakeContext{config: map[string]any{"path": "/tmp/a"}})
	if err == nil || !strings.Contains(err.Error(), "缺少 client 输入") {
		t.Fatalf("期望缺少 client 错误，实际: %v", err)
	}
}

func TestExecuteRequiresPath(t *testing.T) {
	_, err := (Node{}).Execute(fakeContext{
		inputs:  map[string]any{"client": "wrong"},
		handled: map[string]bool{"client": true},
	})
	if err == nil || !strings.Contains(err.Error(), "client 输入类型不是") {
		t.Fatalf("期望 client 类型错误，实际: %v", err)
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
func (c fakeContext) Config(fieldID string) any { return c.config[fieldID] }
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
