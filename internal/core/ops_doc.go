// OpsDoc 是 AI 生成的运维文档资产（巡检/排障/架构等报告）。
// 设计动机（文档 P10）：报告不是一次性 AI 回复，而是可保存、可更新、可导出的文档实体。
//
// 持久化形态（sidecar 模式）：
//   data/docs/<id>.toml  — 元数据（OpsDoc 除 Body 外的字段）
//   data/docs/<id>.md    — Markdown 正文
// 把正文独立成 .md 文件的好处：人类可读、外部工具可编辑、避免 TOML 多行字符串转义。

package core

import "time"

// OpsDocKind 是文档分类枚举，用于前端筛选和后端权限决策。
type OpsDocKind string

const (
	OpsDocKindInspection      OpsDocKind = "inspection"      // 服务器巡检报告
	OpsDocKindTroubleshooting OpsDocKind = "troubleshooting" // 故障排查报告
	OpsDocKindArchitecture    OpsDocKind = "architecture"    // 架构分析文档
)

// OpsDocSource 记录文档的事实来源，用户能追溯到原始执行记录。
// 字段全部可选——架构类文档可能没有 ExecutionID，巡检类文档可能没有 EnvironmentID（无环境的 ad-hoc 工作流）。
type OpsDocSource struct {
	EnvironmentID string `json:"environment_id,omitempty" toml:"environment_id,omitempty"`
	WorkflowID    string `json:"workflow_id,omitempty"    toml:"workflow_id,omitempty"`
	ExecutionID   string `json:"execution_id,omitempty"   toml:"execution_id,omitempty"`
	SessionID     string `json:"session_id,omitempty"     toml:"session_id,omitempty"`
}

// OpsDoc 是一份运维文档资产。
type OpsDoc struct {
	ID        string       `json:"id"         toml:"id"`
	Kind      OpsDocKind   `json:"kind"       toml:"kind"`
	Title     string       `json:"title"      toml:"title"`
	Source    OpsDocSource `json:"source"     toml:"source"`
	Body      string       `json:"body"       toml:"-"` // 持久化时单独写入 <id>.md，不进 TOML
	CreatedAt time.Time    `json:"created_at" toml:"created_at"`
	UpdatedAt time.Time    `json:"updated_at" toml:"updated_at"`
}

// OpsDocSummary 是列表展示用的精简结构（不带 Body）。
type OpsDocSummary struct {
	ID        string       `json:"id"`
	Kind      OpsDocKind   `json:"kind"`
	Title     string       `json:"title"`
	Source    OpsDocSource `json:"source"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// Summary 从 OpsDoc 派生 OpsDocSummary。
func (d OpsDoc) Summary() OpsDocSummary {
	return OpsDocSummary{
		ID: d.ID, Kind: d.Kind, Title: d.Title,
		Source: d.Source, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}
