// 引擎事件推送：统一封装事件名常量和 Emitter 接口
// Emitter 实现可以是 Wails runtime / 测试用 collector / 静默 noop

package engine

import (
	"context"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// 事件名常量，前后端共享约定
const (
	EventStarted  = "execution:started"
	EventStatus   = "execution:status"
	EventNode     = "execution:node"
	EventLog      = "execution:log"
	EventVariable = "execution:variable"
	EventFinished = "execution:finished"
)

// Emitter 推送事件到前端
type Emitter interface {
	Emit(event string, data any)
}

// WailsEmitter 通过 Wails runtime 推送
type WailsEmitter struct {
	ctx context.Context
}

// NewWailsEmitter 创建 Wails 事件推送器
func NewWailsEmitter(ctx context.Context) *WailsEmitter {
	return &WailsEmitter{ctx: ctx}
}

// Emit 推送事件
func (e *WailsEmitter) Emit(event string, data any) {
	if e.ctx == nil {
		return
	}
	wailsruntime.EventsEmit(e.ctx, event, data)
}

// NoopEmitter 静默实现，用于测试或未初始化场景
type NoopEmitter struct{}

// Emit 不做任何事
func (NoopEmitter) Emit(event string, data any) {}
