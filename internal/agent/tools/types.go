// Agent 工具的接口与权限模型。
//
// 设计要点（文档 P7）：
//   - 工具名 = 安全审计点：v1 只暴露命名工具（如 ssh_inspect），不暴露通用命令执行。
//   - 权限分级控制谁能直接调用 vs 必须走工作流确认。
//   - Spec 走 JSON Schema 子集，让 LLM function calling 能消费。
//   - 工具实现层只看到 ToolContext（脱敏的会话信息 + 环境查询回调），不感知 store / Wails。

package tools

import (
	"OpsEngine/internal/core"
)

// PermissionTier 是工具权限分级，决定调用策略。
type PermissionTier string

const (
	// TierRead 只读工具：Agent 可自动调用。例如查看系统信息、列目录、读日志。
	TierRead PermissionTier = "read"
	// TierLowWrite 低风险写入：需要在 prompt 中明确说明。当前未启用。
	TierLowWrite PermissionTier = "low_write"
	// TierHighWrite 高风险变更：必须生成工作流草案、由用户点击确认后执行。当前未启用。
	TierHighWrite PermissionTier = "high_write"
	// TierForbidden 禁止操作：直接拒绝。例如删除根目录、格式化磁盘。当前未启用。
	TierForbidden PermissionTier = "forbidden"
)

// ParamSpec 描述工具一个参数的形状，对应 JSON Schema 子集。
// 这里只支持 LLM 工具调用真正用得到的几种类型，避免在 Schema 层引入完整 JSON Schema 实现。
type ParamSpec struct {
	Type        string `json:"type"`        // "string" / "integer" / "boolean"
	Description string `json:"description"` // 给模型看的描述
	Required    bool   `json:"-"`           // 由 Spec 合成 required 数组
	Default     any    `json:"default,omitempty"`
}

// Spec 是工具对 LLM 公开的元信息。
type Spec struct {
	Name        string               // 函数名，由 LLM 调用时引用
	Description string               // 详细描述，影响模型挑工具的准确率
	Tier        PermissionTier       // 权限级别
	Params      map[string]ParamSpec // 参数 schema
}

// Result 是工具执行结果。
//   - Output 是返回给模型 + 展示给用户的文本（已截断到安全长度）
//   - DisplaySummary 是前端短摘要（可选；缺省取 Output 前 N 字符）
type Result struct {
	Output         string
	DisplaySummary string
}

// ToolContext 是工具实现所需的外部依赖。
// 通过结构体注入而不是 package-level，让单测可以传 stub。
type ToolContext struct {
	SessionID     string
	EnvironmentID string
	// PreferredConfigID 来自会话偏好或 target_select 选定，
	// 工具内部根据需要查找 EnvironmentDef 自行决定目标。
	PreferredConfigID string
	EnvLookup         func(environmentID string) (core.EnvironmentDef, error)
}

// Tool 是工具的运行时接口。Execute 的 args 已由 LLM 的 function calling 协议解析为 map。
type Tool interface {
	Spec() Spec
	Execute(ctx ToolContext, args map[string]any) (Result, error)
}

// MaxOutputBytes 限制单工具输出大小，避免大日志撑爆 LLM 上下文。
const MaxOutputBytes = 4000

// TruncateOutput 把工具输出截到 MaxOutputBytes 以内，超长时附加截断提示。
func TruncateOutput(text string) string {
	if len(text) <= MaxOutputBytes {
		return text
	}
	return text[:MaxOutputBytes] + "\n... (输出已截断)"
}
