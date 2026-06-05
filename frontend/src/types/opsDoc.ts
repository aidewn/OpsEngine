// OpsDoc 文档类型，与后端 core.OpsDoc 字段对齐。

export type OpsDocKind = 'inspection' | 'troubleshooting' | 'architecture';

export interface OpsDocSource {
  environment_id?: string;
  workflow_id?: string;
  execution_id?: string;
  session_id?: string;
}

// OpsDocSummary 列表项（不含 body）。
export interface OpsDocSummary {
  id: string;
  kind: OpsDocKind;
  title: string;
  source: OpsDocSource;
  created_at: string;
  updated_at: string;
}

// OpsDoc 完整文档，body 为 Markdown 字符串。
export interface OpsDoc extends OpsDocSummary {
  body: string;
}
