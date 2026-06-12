package runtime

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestSummarizeArgsValidUTF8 验证含中文的长参数截断后仍是合法 UTF-8。
// 回归：曾因按字节截断中文产生半字符，写进会话 TOML 后导致整个会话加载失败。
func TestSummarizeArgsValidUTF8(t *testing.T) {
	longCN := strings.Repeat("磁盘满了需要清理日志", 10) // 远超 40/80 rune
	got := summarizeArgs(map[string]any{"spec": longCN})
	if !utf8.ValidString(got) {
		t.Fatalf("摘要含非法 UTF-8: %q", got)
	}
	if !strings.Contains(got, "…") {
		t.Fatalf("超长应被截断: %q", got)
	}
}

// TestTruncateRunes 验证 rune 截断不切碎多字节字符。
func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("短", 40); got != "短" {
		t.Fatalf("未超长不应改动: %q", got)
	}
	got := truncateRunes(strings.Repeat("中", 100), 10)
	if !utf8.ValidString(got) || len([]rune(got)) != 10 {
		t.Fatalf("截断结果异常: %q (rune=%d)", got, len([]rune(got)))
	}
}
