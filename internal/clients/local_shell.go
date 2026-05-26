// 本机 Shell 句柄
// 与 LinuxSshClient 形态对齐，但没有持久连接对象：
// 每次执行命令由调用方自行 exec.Command，本句柄只承载「本机执行上下文」的元数据
// 通过 LocalShellConnection 端口在节点之间传递；序列化时仅暴露安全元数据
//
// 未来追加 Run(ctx, cmd) 之类方法时，下游 local_run_command 节点即可像消费
// LinuxSshClient.Sftp() 一样统一调用，无需关心实现细节

package clients

import (
	"encoding/json"
	"os"
	"time"
)

// LocalShellClient 本机 shell 运行时句柄
// 不持有任何资源；Close 是 no-op，便于与 LinuxSshClient 接口风格保持一致
type LocalShellClient struct {
	Host      string    `json:"host"`       // 固定为 "localhost"
	User      string    `json:"user"`       // 当前进程用户（os.Getenv("USER")，兜底空串）
	StartedAt time.Time `json:"started_at"` // 句柄创建时间
}

// NewLocalShellClient 创建本机 shell 句柄
// 不做任何拨号或鉴权 —— 本机执行天然以当前进程身份运行
func NewLocalShellClient() *LocalShellClient {
	return &LocalShellClient{
		Host:      "localhost",
		User:      os.Getenv("USER"),
		StartedAt: time.Now(),
	}
}

// Close 关闭句柄（no-op）
// 保留接口形态，便于上层 defer client.Close() 与 LinuxSshClient 统一书写
func (c *LocalShellClient) Close() error { return nil }

// MarshalJSON 只输出安全元数据，避免误把额外私有字段一起序列化
func (c *LocalShellClient) MarshalJSON() ([]byte, error) {
	if c == nil {
		return []byte("null"), nil
	}
	type safeView struct {
		Type      string    `json:"type"`
		Host      string    `json:"host"`
		User      string    `json:"user"`
		StartedAt time.Time `json:"started_at"`
	}
	return json.Marshal(safeView{
		Type:      "LocalShellConnection",
		Host:      c.Host,
		User:      c.User,
		StartedAt: c.StartedAt,
	})
}
