// 探测接口公共类型
// 用于编辑态「探测一次」（ProbeEnvNode Wails API）与运行态 dynamic 模式共享同一份逻辑

package probe

import "OpsEngine/internal/core"

// ProbeItem 探测结果中的单项（前端 picker 用 / 节点 output 用）
// Key 必须可序列化为字符串，用于 probe_snapshot.picked_key 与 output 端口值
type ProbeItem struct {
	Key   string         `json:"key"`
	Label string         `json:"label"`
	Meta  map[string]any `json:"meta,omitempty"`
}

// ProbeResult 探测返回值
type ProbeResult struct {
	Items []ProbeItem `json:"items"`
}

// ProbeFunc 单个探测节点的真实探测逻辑
// 实现方负责按 env + configID 拉起连接（DialLinuxSsh 等），完成后释放
// nodeConfig 是节点 config 的浅拷贝，可读取 path / pattern 等特有字段
type ProbeFunc func(env core.EnvironmentDef, configID string, nodeConfig map[string]any) (ProbeResult, error)
