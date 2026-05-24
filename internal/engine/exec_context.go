// ExecContext 接口的具体实现
// 每个节点 Execute 调用时引擎构造一个新的实例

package engine

import (
	"context"
	"fmt"

	"OpsEngine/internal/core"
)

// execContextImpl 节点执行上下文，绑定到具体节点、当前图、当前栈和当前 ctx
// ctx 由执行流（goroutine）传入：主流/update/thread 用 runtime.ctx，system_over 用独立 ctx
type execContextImpl struct {
	ctx     context.Context
	runtime *Runtime
	stack   *FrameStack
	node    core.NodeInstance
	nodes   []core.NodeInstance
	edges   []core.EdgeConfig
}

func newExecContext(ctx context.Context, r *Runtime, stack *FrameStack, node core.NodeInstance, nodes []core.NodeInstance, edges []core.EdgeConfig) *execContextImpl {
	return &execContextImpl{
		ctx:     ctx,
		runtime: r,
		stack:   stack,
		node:    node,
		nodes:   nodes,
		edges:   edges,
	}
}

// Context 返回当前流的取消信号，节点 Execute 应监听其 Done 响应取消
func (c *execContextImpl) Context() context.Context { return c.ctx }

// NodeID 当前节点实例 ID
func (c *execContextImpl) NodeID() string { return c.node.InstanceID }

// Input 拉取某 input 端口的值，未连线返回 (nil, false)
func (c *execContextImpl) Input(portID string) (any, bool) {
	return c.runtime.evalInput(c.ctx, c.stack, c.nodes, c.edges, c.node.InstanceID, portID)
}

// Config 读 config 字段原始值
func (c *execContextImpl) Config(fieldID string) any {
	return c.node.Config[fieldID]
}

// ConfigString 读 string 字段，零值 ""
func (c *execContextImpl) ConfigString(fieldID string) string {
	v, _ := c.Config(fieldID).(string)
	return v
}

// ConfigInt 读 int64 字段，零值 0
// 兼容 JSON 反序列化得到的 float64 / int 类型
func (c *execContextImpl) ConfigInt(fieldID string) int64 {
	switch v := c.Config(fieldID).(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		// 容错：用户配的字符串数字
		var n int64
		fmt.Sscanf(v, "%d", &n)
		return n
	}
	return 0
}

// ConfigBool 读 bool 字段，零值 false
func (c *execContextImpl) ConfigBool(fieldID string) bool {
	switch v := c.Config(fieldID).(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	}
	return false
}

// GetVariable 读当前 frame 变量
func (c *execContextImpl) GetVariable(name string) (any, bool) {
	return c.runtime.getVariable(c.stack, name)
}

// SetVariable 写当前 frame 变量
func (c *execContextImpl) SetVariable(name string, value any) {
	c.runtime.setVariable(c.stack, name, value)
}

// GetParam 读当前 frame 的参数
// 仅 assemble frame 有 params；主流返回 (nil, false)
func (c *execContextImpl) GetParam(name string) (any, bool) {
	return c.runtime.getParam(c.stack, name)
}

// Info 写一条 info 日志
func (c *execContextImpl) Info(format string, args ...any) {
	c.runtime.appendLog(c.node.InstanceID, "info", fmt.Sprintf(format, args...))
}

// Warn 写一条 warn 日志
func (c *execContextImpl) Warn(format string, args ...any) {
	c.runtime.appendLog(c.node.InstanceID, "warn", fmt.Sprintf(format, args...))
}

// Error 写一条 error 日志
func (c *execContextImpl) Error(format string, args ...any) {
	c.runtime.appendLog(c.node.InstanceID, "error", fmt.Sprintf(format, args...))
}
