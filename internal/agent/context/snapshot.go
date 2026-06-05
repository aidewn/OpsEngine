// 服务器只读快照采集与过期判定。
// 设计原则：
//   - 快照内容是 hidden=true 的 system 消息，前端不渲染但参与 LLM 调用。
//   - 通过消息内容的固定标记前缀来识别快照，避免在 core.AISession 上新增字段而破坏持久化。
//   - 过期阈值由调用方传入；超过阈值时丢弃旧快照，重新采集。
//
// 后续 P7 工具调用结果也会作为快照写入会话，本文件保留扩展空间。

package agentcontext

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
)

// SnapshotMarker 是 SSH 快照消息的内容前缀，用于在 session.Messages 中识别哪些 hidden system 消息属于快照。
// 注意：变更此常量会让旧会话里的快照无法被识别，等同于强制下一轮重取——可接受。
const SnapshotMarker = "<!-- ops:ssh_snapshot -->"

// DefaultSnapshotTTL 是默认快照有效期。
// 选 10 分钟：足够覆盖单轮排障对话的多条消息，又不至于让用户基于 30 分钟前的 CPU 数值做判断。
const DefaultSnapshotTTL = 10 * time.Minute

// DefaultPrefetchTimeout 是单次 SSH 采集的硬超时。
const DefaultPrefetchTimeout = 20 * time.Second

// EnvironmentLookup 由调用方注入：根据 environmentID 取回完整环境定义。
// 让 context 包不需要直接依赖 store 层。
type EnvironmentLookup func(environmentID string) (core.EnvironmentDef, error)

// LatestSnapshotAt 返回 session 中最新一条 SSH 快照的创建时间。
// 没有快照时返回零值；调用方据此结合 TTL 判断是否需要重取。
func LatestSnapshotAt(session core.AISession) time.Time {
	var latest time.Time
	for _, m := range session.Messages {
		if !isSnapshotMessage(m) {
			continue
		}
		if m.CreatedAt.After(latest) {
			latest = m.CreatedAt
		}
	}
	return latest
}

// SnapshotFresh 判断 session 中是否存在未过期的 SSH 快照。
// ttl <= 0 时视为永不过期（仅用于禁用本特性的测试场景）。
func SnapshotFresh(session core.AISession, ttl time.Duration) bool {
	at := LatestSnapshotAt(session)
	if at.IsZero() {
		return false
	}
	if ttl <= 0 {
		return true
	}
	return time.Since(at) < ttl
}

// PruneSnapshots 返回剔除所有 SSH 快照消息后的 messages 副本，用于在重取前清理旧数据。
// 原 slice 不变。
func PruneSnapshots(messages []core.AISessionMessage) []core.AISessionMessage {
	out := make([]core.AISessionMessage, 0, len(messages))
	for _, m := range messages {
		if isSnapshotMessage(m) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// Prefetch 通过 SSH 跑只读巡检脚本，把结果包装成带 SnapshotMarker 前缀的字符串。
// 调用方应把返回字符串作为 hidden=true 的 system 消息追加到会话。
func Prefetch(envLookup EnvironmentLookup, environmentID, configID string, timeout time.Duration) (string, error) {
	if envLookup == nil {
		return "", errors.New("environment lookup 未注入")
	}
	if timeout <= 0 {
		timeout = DefaultPrefetchTimeout
	}
	env, err := envLookup(environmentID)
	if err != nil {
		return "", err
	}
	var fields map[string]any
	for _, c := range env.Configs {
		if c.ID == configID {
			fields = c.Fields
			break
		}
	}
	if fields == nil {
		return "", fmt.Errorf("环境 %s 中未找到配置 %s", environmentID, configID)
	}
	dial, err := clients.ParseLinuxSshDialConfig(fields)
	if err != nil {
		return "", err
	}
	client, err := dial.Dial()
	if err != nil {
		return "", err
	}
	defer client.Close()

	session, err := client.Client().NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := session.CombinedOutput(prefetchScript)
		done <- result{out, err}
	}()

	select {
	case r := <-done:
		body := truncateOutput(string(r.out), 12000)
		// 命令非零退出不致命，仍把 stdout 暴露给模型。
		return formatSnapshot(env.Name, configID, body), nil
	case <-time.After(timeout):
		_ = session.Close()
		return "", fmt.Errorf("采集超时（%s）", timeout)
	}
}

// formatSnapshot 把巡检输出包装成带 marker 的 Markdown 块。
// marker 在首行使得 isSnapshotMessage 可以零成本识别。
func formatSnapshot(envName, configID, body string) string {
	header := fmt.Sprintf("以下是目标服务器（环境 %s / 配置 %s）当前的真实状态，回答用户问题时请优先参考这些数据：\n\n", envName, configID)
	return SnapshotMarker + "\n" + header + "```\n" + body + "\n```"
}

// isSnapshotMessage 判断一条消息是否为 SSH 快照（hidden system + SnapshotMarker 前缀）。
func isSnapshotMessage(m core.AISessionMessage) bool {
	if m.Role != core.AIMessageRoleSystem || !m.Hidden {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(m.Content), SnapshotMarker)
}

// truncateOutput 限制单次注入 prompt 的字节数，避免吃满上下文窗口。
func truncateOutput(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	return text[:maxBytes] + "\n... (输出已截断)"
}

// prefetchScript 是只读巡检脚本，覆盖 OS / 负载 / 内存 / 磁盘 / 进程 / 网络。
// 每行都加 2>/dev/null 兼容缺工具的最小镜像。
const prefetchScript = `
set +e
echo "## 系统信息"
uname -a 2>/dev/null
(cat /etc/os-release 2>/dev/null || cat /etc/issue 2>/dev/null) | head -10
echo
echo "## 运行时长与负载"
uptime 2>/dev/null
echo
echo "## CPU"
(lscpu 2>/dev/null | head -15) || cat /proc/cpuinfo 2>/dev/null | head -10
echo
echo "## 内存"
(free -h 2>/dev/null) || (head -5 /proc/meminfo 2>/dev/null)
echo
echo "## 磁盘"
df -hT 2>/dev/null
echo
echo "## CPU 占用前 10 进程"
ps -eo pid,user,pcpu,pmem,comm --sort=-pcpu 2>/dev/null | head -11
echo
echo "## 网卡地址"
(ip -br addr 2>/dev/null) || (ifconfig 2>/dev/null | head -20)
echo
echo "## 监听端口"
(ss -tln 2>/dev/null | head -20) || (netstat -tln 2>/dev/null | head -20)
`
