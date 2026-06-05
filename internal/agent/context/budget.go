// 长会话裁剪：保证投喂给 LLM 的消息数有上界，避免上下文窗口溢出。
// 当前策略很简单：保留全部 system 消息（含 SSH 快照）+ 最近 N 轮 user/assistant 对话。
// P4 后续可升级为"对超出部分生成 summary 摘要"，本文件留好挂载点。

package agentcontext

import "OpsEngine/internal/core"

// DefaultMaxTurns 是默认保留的对话轮次（一对 user+assistant 计为 1 轮）。
// 选 12 轮：约对应 24 条消息，足以承载一次完整排障，又不会超 DeepSeek 64K 上下文。
const DefaultMaxTurns = 12

// TruncateMessages 按"系统消息全留 + 最近 maxTurns 对 user/assistant"裁剪。
// maxTurns <= 0 时不裁剪，原样返回。
//
// 实现思路：
//  1. 倒序扫描，凑齐 maxTurns 个 user 消息对应的窗口位置。
//  2. 该窗口之外的非 system 消息丢弃。
//  3. system 消息无论位置在哪都保留——SSH 快照等关键事实不能丢。
func TruncateMessages(messages []core.AISessionMessage, maxTurns int) []core.AISessionMessage {
	if maxTurns <= 0 || len(messages) == 0 {
		return messages
	}
	// 从尾部往前数 user 消息，确定保留窗口的起始下标。
	userSeen := 0
	cutoff := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == core.AIMessageRoleUser {
			userSeen++
			if userSeen >= maxTurns {
				cutoff = i
				break
			}
		}
	}
	if userSeen < maxTurns {
		// user 消息数还没超阈值，无需裁剪。
		return messages
	}
	out := make([]core.AISessionMessage, 0, len(messages))
	for i, m := range messages {
		if i >= cutoff {
			out = append(out, m)
			continue
		}
		// 窗口之前只保留 system 消息（含 SSH 快照），保证事实不丢失。
		if m.Role == core.AIMessageRoleSystem {
			out = append(out, m)
		}
	}
	return out
}
