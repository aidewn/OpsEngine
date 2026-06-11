// execsum 包单元测试：验证失败节点收集、嵌套 frame 遍历与截断行为。
package execsum

import (
	"strings"
	"testing"
	"time"

	"OpsEngine/internal/core"
)

// makeFailedRecord 构造一条含主流失败节点 + 集合内失败节点的执行记录。
func makeFailedRecord() core.ExecutionRecord {
	longVal := strings.Repeat("x", 2000)
	child := &core.FrameState{
		AssembleID: "asm-1",
		NodeStates: map[string]core.NodeState{"a1": core.NodeStateFailed},
		NodeLogs: map[string][]core.LogEntry{
			"a1": {{Time: time.Now(), Level: "error", Message: "sed: unmatched delimiter"}},
		},
		Variables: map[string]any{"path": "/opt/app"},
	}
	return core.ExecutionRecord{
		ID:         "exec-1",
		WorkflowID: "wf-1",
		Status:     core.WorkflowStatusFailed,
		Snapshot: core.ExecutionSnapshot{
			Workflow: core.WorkflowDef{
				ID: "wf-1", Name: "部署",
				Nodes: []core.NodeInstance{
					{InstanceID: "n1", TypeID: "linux_exec_command", Config: map[string]any{"command": "deploy.sh"}},
					{InstanceID: "n2", TypeID: "system_ready"},
				},
			},
			Assembles: map[string]core.AssembleDef{
				"asm-1": {ID: "asm-1", Name: "重启服务", Nodes: []core.NodeInstance{
					{InstanceID: "a1", TypeID: "linux_exec_script", Config: map[string]any{"script": "restart.sh"}},
				}},
			},
		},
		RootFrame: core.FrameState{
			NodeStates: map[string]core.NodeState{
				"n1": core.NodeStateFailed,
				"n2": core.NodeStateSuccess,
			},
			NodeLogs: map[string][]core.LogEntry{
				"n1": {
					{Time: time.Now(), Level: "info", Message: "start"},
					{Time: time.Now(), Level: "error", Message: "exit_code=1"},
				},
			},
			Variables: map[string]any{"big": longVal},
			Children:  map[string]*core.FrameState{"caller-1": child},
		},
	}
}

// TestCollectFailedNodes 验证主流与嵌套集合的失败节点都被收集且带正确上下文。
func TestCollectFailedNodes(t *testing.T) {
	failed := CollectFailedNodes(makeFailedRecord())
	if len(failed) != 2 {
		t.Fatalf("应收集 2 个失败节点，得到 %d: %#v", len(failed), failed)
	}
	byID := map[string]FailedNode{}
	for _, f := range failed {
		byID[f.InstanceID] = f
	}
	n1, ok := byID["n1"]
	if !ok || n1.TypeID != "linux_exec_command" || n1.FramePath != "main" {
		t.Fatalf("主流失败节点异常: %#v", n1)
	}
	if len(n1.Logs) != 2 || n1.Logs[1].Message != "exit_code=1" {
		t.Fatalf("主流节点日志异常: %#v", n1.Logs)
	}
	a1, ok := byID["a1"]
	if !ok || a1.TypeID != "linux_exec_script" || a1.FramePath != "main > 重启服务" {
		t.Fatalf("集合内失败节点异常: %#v", a1)
	}
}

// TestFailureContextTruncation 验证超长变量值被截断且整体输出有界。
func TestFailureContextTruncation(t *testing.T) {
	text := FailureContext(makeFailedRecord())
	if !strings.Contains(text, "exit_code=1") || !strings.Contains(text, "sed: unmatched delimiter") {
		t.Fatalf("失败日志缺失:\n%s", text)
	}
	if !strings.Contains(text, "…(已截断)") {
		t.Fatalf("超长变量未截断:\n%s", text)
	}
	if len([]rune(text)) > maxTotalLen+20 {
		t.Fatalf("整体输出超过上限: %d", len([]rune(text)))
	}
}

// TestTailLogs 验证日志只保留尾部 maxLogsPerNode 条。
func TestTailLogs(t *testing.T) {
	logs := make([]core.LogEntry, 30)
	for i := range logs {
		logs[i] = core.LogEntry{Message: strings.Repeat("m", 400)}
	}
	out := tailLogs(logs)
	if len(out) != maxLogsPerNode {
		t.Fatalf("日志条数 = %d, want %d", len(out), maxLogsPerNode)
	}
	if len([]rune(out[0].Message)) > maxLogLineLen+10 {
		t.Fatalf("单条日志未截断: %d", len(out[0].Message))
	}
}

// TestSummarySuccess 验证成功执行只输出概要不带失败段。
func TestSummarySuccess(t *testing.T) {
	rec := makeFailedRecord()
	rec.Status = core.WorkflowStatusSuccess
	text := Summary(rec)
	if strings.Contains(text, "## 失败节点") {
		t.Fatalf("成功执行不应包含失败详情段:\n%s", text)
	}
}
