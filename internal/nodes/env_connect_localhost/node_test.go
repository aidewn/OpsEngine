// env_connect_localhost 单元测试
// 覆盖 TypeDef 关键字段、缺配置错误路径、以及成功路径下返回 *clients.LocalShellClient

package env_connect_localhost

import (
	"context"
	"strings"
	"testing"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	"OpsEngine/internal/store"
)

// 验证 TypeDef 关键字段：TypeID / NodeKind / 输出端口形态
func TestTypeDef(t *testing.T) {
	def := (Node{}).TypeDef()
	if def.TypeID != "env_connect_localhost" {
		t.Fatalf("TypeID 不匹配: %s", def.TypeID)
	}
	if def.NodeKind != core.NodeKindAction {
		t.Fatalf("NodeKind 不匹配: %s", def.NodeKind)
	}
	if len(def.OutputPorts) != 2 {
		t.Fatalf("OutputPorts 数量不匹配: %d", len(def.OutputPorts))
	}
	if def.OutputPorts[1].ID != "client" || def.OutputPorts[1].PortType != core.PortTypeLocalShell {
		t.Fatalf("client 输出端口定义不匹配: %+v", def.OutputPorts[1])
	}
}

// 没有 environment_id 直接报错
func TestExecuteRequiresEnvironmentID(t *testing.T) {
	_, err := (Node{}).Execute(fakeContext{config: map[string]any{}})
	if err == nil || !strings.Contains(err.Error(), "environment_id") {
		t.Fatalf("期望 environment_id 错误，实际: %v", err)
	}
}

// 没有 config_id 直接报错
func TestExecuteRequiresConfigID(t *testing.T) {
	_, err := (Node{}).Execute(fakeContext{config: map[string]any{
		"environment_id": "env-1",
	}})
	if err == nil || !strings.Contains(err.Error(), "config_id") {
		t.Fatalf("期望 config_id 错误，实际: %v", err)
	}
}

// 引擎未注入 environmentStore：必须显式报错而非空指针
func TestExecuteRequiresEnvironmentStore(t *testing.T) {
	_, err := (Node{}).Execute(fakeContext{
		config: map[string]any{
			"environment_id": "env-1",
			"config_id":      "local-1",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "environmentStore") {
		t.Fatalf("期望 environmentStore 错误，实际: %v", err)
	}
}

// kind 错配（如指向 ssh 配置）：必须明确报错
func TestExecuteRejectsNonLocalhostConfig(t *testing.T) {
	es := store.NewEnvironmentStore(t.TempDir())
	if err := es.Save(core.EnvironmentDef{
		ID: "env-1", Name: "e",
		Configs: []core.EnvConfigItem{
			{ID: "wrong", Kind: core.EnvConfigKindSSH, Name: "x"},
		},
	}); err != nil {
		t.Fatalf("Save env 失败: %v", err)
	}
	_, err := (Node{}).Execute(fakeContext{
		config: map[string]any{
			"environment_id": "env-1",
			"config_id":      "wrong",
		},
		envStore: es,
	})
	if err == nil || !strings.Contains(err.Error(), "不是 localhost") {
		t.Fatalf("期望「不是 localhost」错误，实际: %v", err)
	}
}

// 正常路径：返回 *clients.LocalShellClient 实例
func TestExecuteReturnsLocalShellClient(t *testing.T) {
	es := store.NewEnvironmentStore(t.TempDir())
	if err := es.Save(core.EnvironmentDef{
		ID: "env-1", Name: "e",
		Configs: []core.EnvConfigItem{
			{ID: "local-1", Kind: core.EnvConfigKindLocalhost, Name: "本机"},
		},
	}); err != nil {
		t.Fatalf("Save env 失败: %v", err)
	}
	out, err := (Node{}).Execute(fakeContext{
		config: map[string]any{
			"environment_id": "env-1",
			"config_id":      "local-1",
		},
		envStore: es,
	})
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	cli, ok := out["client"].(*clients.LocalShellClient)
	if !ok || cli == nil {
		t.Fatalf("client 不是 *LocalShellClient: %T", out["client"])
	}
	if cli.Host != "localhost" {
		t.Fatalf("Host 不是 localhost: %s", cli.Host)
	}
}

// fakeContext 仅实现节点用到的方法 + EnvironmentStore() 兜底
// 其余方法返回零值以满足 engine.ExecContext 接口
type fakeContext struct {
	config   map[string]any
	envStore *store.EnvironmentStore
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
func (c fakeContext) GetVariable(string) (any, bool)            { return nil, false }
func (c fakeContext) SetVariable(string, any)                   {}
func (c fakeContext) GetParam(string) (any, bool)               { return nil, false }
func (c fakeContext) SetReturn(string, any)                     {}
func (c fakeContext) Info(string, ...any)                       {}
func (c fakeContext) Warn(string, ...any)                       {}
func (c fakeContext) Error(string, ...any)                      {}
func (c fakeContext) EnvironmentStore() *store.EnvironmentStore { return c.envStore }

var _ engine.ExecContext = fakeContext{}
