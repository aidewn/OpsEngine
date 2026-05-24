// 节点开发者面对的契约：实现 Node 接口即可注册到引擎
// Execute 由引擎调用；Pure 节点也实现 Execute，由 evaluator 按需触发

package engine

import (
	"context"

	"OpsEngine/internal/core"
)

// Outputs 节点输出端口的值
// key = output port ID（如 "exec_out"、"value"、"return_<name>"）
type Outputs map[string]any

// Node 节点逻辑实现，每个节点类型对应一个 Node 实例
type Node interface {
	// TypeDef 返回节点的元信息（端口、config schema、UI 展示等）
	TypeDef() core.NodeTypeDef

	// Execute 执行节点逻辑
	// Action 节点：由引擎沿 exec 流推进时调用
	// Pure 节点：由 evaluator 按需求值时调用，结果通常不缓存
	// Event 节点：作为 exec 流起点，Execute 通常为空（仅作占位）
	Execute(ctx ExecContext) (Outputs, error)
}

// ExecContext 节点 Execute 时拿到的上下文
// 通过它访问输入、配置、变量、日志、取消信号
type ExecContext interface {
	// Context 用于响应取消（用户点停止 / 工作流终止）
	Context() context.Context

	// NodeID 当前节点的实例 ID
	NodeID() string

	// Input 拉取某个 input 端口的值
	// 内部按需求值上游 pure 节点 / 读已执行 action 的 output cache
	// 未连线时返回 (类型零值, false)
	Input(portID string) (any, bool)

	// Config 读节点 config 的原始值
	Config(fieldID string) any

	// ConfigString / ConfigInt / ConfigBool 类型化便利方法
	// 字段不存在或类型不匹配时返回零值
	ConfigString(fieldID string) string
	ConfigInt(fieldID string) int64
	ConfigBool(fieldID string) bool

	// GetVariable 读当前 frame 作用域的变量
	GetVariable(name string) (any, bool)

	// SetVariable 写当前 frame 作用域的变量
	SetVariable(name string, value any)

	// GetParam 读当前 frame 的参数值
	// 仅 assemble frame 有 params；主流 frame 无 params，返回 (nil, false)
	GetParam(name string) (any, bool)

	// 日志（推送到前端 execution:log 事件）
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
}
