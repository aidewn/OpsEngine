// ExecContext 接口的具体实现
// 每个节点 Execute 调用时引擎构造一个新的实例，绑定到当前 frame

package engine

import (
	"context"
	"fmt"

	"OpsEngine/internal/core"
)

// execContextImpl 节点执行上下文，绑定到具体节点、当前图、当前 frame、当前 ctx
type execContextImpl struct {
	ctx     context.Context
	runtime *Runtime
	frame   *Frame
	node    core.NodeInstance
	nodes   []core.NodeInstance
	edges   []core.EdgeConfig
}

func newExecContext(ctx context.Context, r *Runtime, frame *Frame, node core.NodeInstance, nodes []core.NodeInstance, edges []core.EdgeConfig) *execContextImpl {
	return &execContextImpl{
		ctx:     ctx,
		runtime: r,
		frame:   frame,
		node:    node,
		nodes:   nodes,
		edges:   edges,
	}
}

func (c *execContextImpl) Context() context.Context { return c.ctx }
func (c *execContextImpl) NodeID() string           { return c.node.InstanceID }

func (c *execContextImpl) Input(portID string) (any, bool) {
	return c.runtime.evalInput(c.ctx, c.frame, c.nodes, c.edges, c.node.InstanceID, portID)
}

func (c *execContextImpl) Config(fieldID string) any {
	return c.node.Config[fieldID]
}

func (c *execContextImpl) ConfigString(fieldID string) string {
	v, _ := c.Config(fieldID).(string)
	return v
}

func (c *execContextImpl) ConfigInt(fieldID string) int64 {
	switch v := c.Config(fieldID).(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		var n int64
		fmt.Sscanf(v, "%d", &n)
		return n
	}
	return 0
}

func (c *execContextImpl) ConfigBool(fieldID string) bool {
	switch v := c.Config(fieldID).(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	}
	return false
}

func (c *execContextImpl) GetVariable(name string) (any, bool) {
	return c.runtime.getVariable(c.frame, name)
}

func (c *execContextImpl) SetVariable(name string, value any) {
	c.runtime.setVariable(c.frame, name, value)
}

func (c *execContextImpl) GetParam(name string) (any, bool) {
	return c.runtime.getParam(c.frame, name)
}

func (c *execContextImpl) SetReturn(name string, value any) {
	c.runtime.setReturn(c.frame, name, value)
}

func (c *execContextImpl) Info(format string, args ...any) {
	c.runtime.appendLog(c.frame, c.node.InstanceID, "info", fmt.Sprintf(format, args...))
}

func (c *execContextImpl) Warn(format string, args ...any) {
	c.runtime.appendLog(c.frame, c.node.InstanceID, "warn", fmt.Sprintf(format, args...))
}

func (c *execContextImpl) Error(format string, args ...any) {
	c.runtime.appendLog(c.frame, c.node.InstanceID, "error", fmt.Sprintf(format, args...))
}
