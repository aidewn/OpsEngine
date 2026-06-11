// streamlog 单元测试：切行、CR/ANSI 清洗、批量、限量。
package streamlog

import (
	"strings"
	"testing"
)

// collect 返回写入器和已回调内容的收集器。
func collect() (*Writer, *[]string) {
	out := &[]string{}
	w := New(func(chunk string) { *out = append(*out, chunk) })
	return w, out
}

// TestLineSplitAndFlush 验证按 \n 切行与 Flush 冲刷残余半行。
func TestLineSplitAndFlush(t *testing.T) {
	w, out := collect()
	_, _ = w.Write([]byte("line1\nline2\npart"))
	w.Flush()
	joined := strings.Join(*out, "\n")
	for _, want := range []string{"line1", "line2", "part"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("缺少 %q: %v", want, *out)
		}
	}
}

// TestCRAndANSIClean 验证 \r 进度条切行与 ANSI 序列剥离。
func TestCRAndANSIClean(t *testing.T) {
	w, out := collect()
	_, _ = w.Write([]byte("下载中 10%\r下载中 50%\r\x1b[32m完成\x1b[0m\n"))
	w.Flush()
	joined := strings.Join(*out, "\n")
	if !strings.Contains(joined, "下载中 10%") || !strings.Contains(joined, "完成") {
		t.Fatalf("CR 切行/清洗异常: %v", *out)
	}
	if strings.Contains(joined, "\x1b") {
		t.Fatalf("ANSI 序列未剥离: %q", joined)
	}
}

// TestStreamCap 验证超过 maxStreamLines 后停止推送并打一次提示。
func TestStreamCap(t *testing.T) {
	w, out := collect()
	for i := 0; i < maxStreamLines+100; i++ {
		_, _ = w.Write([]byte("x\n"))
	}
	w.Flush()
	total := 0
	capNotice := 0
	for _, chunk := range *out {
		for _, line := range strings.Split(chunk, "\n") {
			if strings.Contains(line, "实时输出已达上限") {
				capNotice++
				continue
			}
			total++
		}
	}
	if total != maxStreamLines {
		t.Fatalf("推送行数应为 %d，got %d", maxStreamLines, total)
	}
	if capNotice != 1 {
		t.Fatalf("上限提示应恰好一次，got %d", capNotice)
	}
}

// TestLongLineTruncate 验证超长行截断。
func TestLongLineTruncate(t *testing.T) {
	w, out := collect()
	_, _ = w.Write([]byte(strings.Repeat("a", maxLineLen+100) + "\n"))
	w.Flush()
	if len(*out) == 0 || !strings.HasSuffix((*out)[0], "…") {
		t.Fatalf("超长行应截断: %v", *out)
	}
}
