// Context Manager 单测：覆盖快照识别、过期判定、剪除、长会话裁剪。

package agentcontext

import (
	"strings"
	"testing"
	"time"

	"OpsEngine/internal/core"
)

// newSnapshotMsg 构造一条带 SnapshotMarker 的 hidden system 消息。
func newSnapshotMsg(at time.Time) core.AISessionMessage {
	return core.AISessionMessage{
		Role:      core.AIMessageRoleSystem,
		Hidden:    true,
		Content:   SnapshotMarker + "\nCPU: 20%",
		CreatedAt: at,
	}
}

// TestSnapshotFresh 验证 TTL 内识别为 fresh，超出阈值识别为过期。
func TestSnapshotFresh(t *testing.T) {
	now := time.Now()
	session := core.AISession{Messages: []core.AISessionMessage{
		newSnapshotMsg(now.Add(-2 * time.Minute)),
	}}
	if !SnapshotFresh(session, 10*time.Minute) {
		t.Fatal("2 分钟前的快照应判为 fresh")
	}
	if SnapshotFresh(session, 1*time.Minute) {
		t.Fatal("1 分钟 TTL 下 2 分钟前的快照应判为过期")
	}
}

// TestSnapshotFreshNoSnapshot 验证无快照时返回 false。
func TestSnapshotFreshNoSnapshot(t *testing.T) {
	session := core.AISession{Messages: []core.AISessionMessage{
		{Role: core.AIMessageRoleUser, Content: "hi"},
	}}
	if SnapshotFresh(session, 10*time.Minute) {
		t.Fatal("空会话不应判为 fresh")
	}
}

// TestPruneSnapshots 验证仅剔除快照消息，其他消息原样保留。
func TestPruneSnapshots(t *testing.T) {
	now := time.Now()
	msgs := []core.AISessionMessage{
		newSnapshotMsg(now.Add(-time.Hour)),
		{Role: core.AIMessageRoleUser, Content: "hi"},
		{Role: core.AIMessageRoleSystem, Hidden: true, Content: "其他 system 消息（不带 marker）"},
	}
	pruned := PruneSnapshots(msgs)
	if len(pruned) != 2 {
		t.Fatalf("剔除后应剩 2 条，实际 %d", len(pruned))
	}
	for _, m := range pruned {
		if strings.HasPrefix(strings.TrimSpace(m.Content), SnapshotMarker) {
			t.Fatal("快照未被剔除")
		}
	}
}

// TestTruncateMessagesKeepsSystem 验证裁剪保留全部 system 消息 + 最近 N 轮对话。
func TestTruncateMessagesKeepsSystem(t *testing.T) {
	msgs := []core.AISessionMessage{
		{Role: core.AIMessageRoleSystem, Content: "snapshot"},
		{Role: core.AIMessageRoleUser, Content: "u1"},
		{Role: core.AIMessageRoleAssistant, Content: "a1"},
		{Role: core.AIMessageRoleUser, Content: "u2"},
		{Role: core.AIMessageRoleAssistant, Content: "a2"},
		{Role: core.AIMessageRoleUser, Content: "u3"},
		{Role: core.AIMessageRoleAssistant, Content: "a3"},
	}
	out := TruncateMessages(msgs, 2)
	// 期望：保留 system + 最后 2 对（u2/a2/u3/a3）。
	if len(out) != 5 {
		t.Fatalf("expected 5, got %d: %#v", len(out), out)
	}
	if out[0].Role != core.AIMessageRoleSystem {
		t.Fatalf("首条应为 system: %#v", out[0])
	}
	if out[1].Content != "u2" || out[4].Content != "a3" {
		t.Fatalf("窗口范围错误: %#v", out)
	}
}

// TestTruncateMessagesBelowThreshold 验证未超阈值时原样返回。
func TestTruncateMessagesBelowThreshold(t *testing.T) {
	msgs := []core.AISessionMessage{
		{Role: core.AIMessageRoleUser, Content: "u1"},
		{Role: core.AIMessageRoleAssistant, Content: "a1"},
	}
	out := TruncateMessages(msgs, 5)
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
}

// TestTruncateMessagesZeroDisabled 验证 maxTurns<=0 时禁用裁剪。
func TestTruncateMessagesZeroDisabled(t *testing.T) {
	msgs := []core.AISessionMessage{
		{Role: core.AIMessageRoleUser, Content: "u1"},
		{Role: core.AIMessageRoleUser, Content: "u2"},
	}
	out := TruncateMessages(msgs, 0)
	if len(out) != 2 {
		t.Fatalf("maxTurns=0 应禁用裁剪")
	}
}
