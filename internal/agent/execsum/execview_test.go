package execsum

import (
	"testing"
	"time"

	"OpsEngine/internal/core"
)

// TestBuildExecutionView 验证执行视图提取状态与失败节点（含最后一条日志）。
func TestBuildExecutionView(t *testing.T) {
	rec := core.ExecutionRecord{
		ID:         "ex-1",
		WorkflowID: "wf-1",
		Status:     core.WorkflowStatusFailed,
		Error:      "整体失败",
		Snapshot: core.ExecutionSnapshot{
			Workflow: core.WorkflowDef{
				Name:  "部署",
				Nodes: []core.NodeInstance{{InstanceID: "n1", TypeID: "linux_exec_command"}},
			},
		},
		RootFrame: core.FrameState{
			NodeStates: map[string]core.NodeState{"n1": core.NodeStateFailed},
			NodeLogs: map[string][]core.LogEntry{
				"n1": {{Time: time.Now(), Level: "error", Message: "exit code 1"}},
			},
		},
	}
	v := BuildExecutionView(rec)
	if v.Status != "Failed" || v.WorkflowName != "部署" || v.Error != "整体失败" {
		t.Fatalf("基础字段异常: %#v", v)
	}
	if len(v.FailedNodes) != 1 {
		t.Fatalf("应有 1 个失败节点: %#v", v.FailedNodes)
	}
	n := v.FailedNodes[0]
	if n.InstanceID != "n1" || n.TypeID != "linux_exec_command" || n.LastLog != "exit code 1" {
		t.Fatalf("失败节点字段异常: %#v", n)
	}
}
