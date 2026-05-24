// 调用栈 per-goroutine 视图
// 设计：
//   - 每个执行流（goroutine）持有独立的 FrameStack
//   - 栈底是 mainFrame（所有栈共享同一个对象）
//   - push 集合 frame 时只修改当前栈的 slice，不影响其他栈
//   - Frame 内部的 Variables / Params / Returns map 仍然可被多个栈共享，
//     变更通过 Runtime.mu 串行化

package engine

// FrameStack 单个执行流的调用栈
type FrameStack struct {
	frames []*Frame
}

// newStack 用根帧（主帧）创建一个栈
func newStack(mainFrame *Frame) *FrameStack {
	return &FrameStack{frames: []*Frame{mainFrame}}
}

// push 进入一层调用
func (s *FrameStack) push(f *Frame) {
	s.frames = append(s.frames, f)
}

// pop 退出当前层调用
func (s *FrameStack) pop() {
	if len(s.frames) > 0 {
		s.frames = s.frames[:len(s.frames)-1]
	}
}

// current 栈顶 frame（当前作用域）
func (s *FrameStack) current() *Frame {
	if len(s.frames) == 0 {
		return nil
	}
	return s.frames[len(s.frames)-1]
}

// fork 复制当前栈作为子执行流的起点
// 复制 slice，frames 元素仍是共享引用（变量同步通过 mu 保护）
func (s *FrameStack) fork() *FrameStack {
	cp := make([]*Frame, len(s.frames))
	copy(cp, s.frames)
	return &FrameStack{frames: cp}
}
