// streamlog 把命令的实时输出流转成节点日志：切行、清洗、限流、限量。
// 用法：节点把 Writer 接到 session.Stdout/Stderr（与完整输出 buffer 组成 MultiWriter），
// 输出边产生边按批回调日志函数，长任务（安装/编译）的过程因此实时可见。
//
// 三道防护（讨论结论）：
//   - 批量 flush：每 flushInterval 或攒满 maxBatchLines 行才回调一次，避免事件风暴
//   - 行数上限：超过 maxStreamLines 后停止实时推送（完整输出仍在节点 outputs 里）
//   - CR/ANSI 清洗：apt/yum 进度条按 \r 原地刷新并夹带控制序列，按 \r 切行并剥离 ANSI
package streamlog

import (
	"bytes"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	// flushInterval 两次日志回调的最小间隔。
	flushInterval = 200 * time.Millisecond
	// maxBatchLines 单次回调最多携带的行数，攒满立即 flush。
	maxBatchLines = 50
	// maxStreamLines 单个 Writer 实时推送的总行数上限。
	maxStreamLines = 500
	// maxLineLen 单行最大字符数，超长截断（进度条清洗失败时的兜底）。
	maxLineLen = 500
)

// ansiPattern 匹配 ANSI 控制序列（颜色/光标移动）。
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

// Writer 是 io.Writer，把写入的字节流切行后批量回调 logf。
// 并发安全：stdout/stderr 各建一个实例即可，互不共享。
type Writer struct {
	mu        sync.Mutex
	logf      func(chunk string) // 每批调用一次，chunk 为多行文本
	partial   bytes.Buffer       // 未凑满一行的残余
	pending   []string           // 已切出待 flush 的行
	lastFlush time.Time
	lines     int  // 已推送的总行数
	capped    bool // 是否已触达上限并打过提示
}

// New 创建流式日志写入器。logf 通常是 ctx.Info / ctx.Warn 的包装。
func New(logf func(chunk string)) *Writer {
	return &Writer{logf: logf}
}

// Write 实现 io.Writer：永不报错（日志通道故障不应中断命令本体）。
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.partial.Write(p)
	w.drainLines()
	if len(w.pending) >= maxBatchLines || time.Since(w.lastFlush) >= flushInterval {
		w.flushLocked()
	}
	return len(p), nil
}

// Flush 命令结束后调用：冲刷残余的半行与未发送批次。
func (w *Writer) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if rest := strings.TrimSpace(w.partial.String()); rest != "" {
		w.appendLine(rest)
		w.partial.Reset()
	}
	w.flushLocked()
}

// drainLines 把 partial 中的完整行（\n 或 \r 结尾）切出到 pending。
func (w *Writer) drainLines() {
	data := w.partial.Bytes()
	start := 0
	for i, b := range data {
		if b != '\n' && b != '\r' {
			continue
		}
		if line := strings.TrimSpace(string(data[start:i])); line != "" {
			w.appendLine(line)
		}
		start = i + 1
	}
	if start > 0 {
		rest := append([]byte(nil), data[start:]...)
		w.partial.Reset()
		w.partial.Write(rest)
	}
}

// appendLine 清洗单行并计入限量。
func (w *Writer) appendLine(line string) {
	if w.lines >= maxStreamLines {
		if !w.capped {
			w.capped = true
			w.pending = append(w.pending, "…(实时输出已达上限，后续省略；完整输出见节点 stdout/stderr)")
		}
		return
	}
	line = ansiPattern.ReplaceAllString(line, "")
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if runes := []rune(line); len(runes) > maxLineLen {
		line = string(runes[:maxLineLen]) + "…"
	}
	w.pending = append(w.pending, line)
	w.lines++
}

// flushLocked 把 pending 合并成一条多行日志回调出去。调用方必须已持锁。
func (w *Writer) flushLocked() {
	if len(w.pending) == 0 {
		return
	}
	w.logf(strings.Join(w.pending, "\n"))
	w.pending = w.pending[:0]
	w.lastFlush = time.Now()
}
