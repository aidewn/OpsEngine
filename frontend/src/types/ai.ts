// AI 设置、会话与统一助手类型，和 Go 侧 Wails 绑定字段保持一致。

// AISettings 保存大模型连接配置。字段名沿用 deepseek_*，但 base_url 可换成任何
// OpenAI 兼容端点（DeepSeek / OpenAI / 本地 ollama 等）。
export interface AISettings {
  /** API Key。 */
  deepseek_api_key: string;
  /** OpenAI 兼容接口地址。 */
  deepseek_base_url: string;
  /** 聊天模型名称。 */
  deepseek_model: string;
  /** 单次请求超时时间。 */
  timeout_seconds: number;
  /** AI 修改已有资产的落盘策略：confirm（默认，先确认）/ auto（直接保存）。 */
  apply_mode: 'confirm' | 'auto';
}

// AIViewPayload 是 Agent 工具产出的可视化载荷（与后端 core.AIViewPayload 对齐）。
// data 是视图专属 JSON 字符串，由 ViewRenderer 按 kind 解析渲染。
export interface AIViewPayload {
  kind: string;
  title: string;
  data: string;
}

// AISessionMessageRole 是会话消息的角色枚举。
export type AISessionMessageRole = 'system' | 'user' | 'assistant';

// AISessionMessage 是一条会话消息（与后端 core.AISessionMessage 对齐）。
export interface AISessionMessage {
  id: string;
  role: AISessionMessageRole;
  content: string;
  progress?: string[];
  workflow_id?: string;
  workflow_name?: string;
  assemble_id?: string;
  assemble_name?: string;
  artifact_type?: 'assemble' | 'workflow' | 'doc';
  action_type?: 'create' | 'update';
  /** 指向消息产生时落地的 OpsDoc，前端用于"查看文档"跳转。 */
  doc_id?: string;
  doc_title?: string;
  /** 隐藏消息（预取的服务器状态等），前端不渲染。 */
  hidden?: boolean;
  /** 产生该消息时的意图标签（'chat' / 'troubleshoot' / 'inspect_server' / ...），用于决定是否显示"保存为报告"等动作。 */
  intent?: string;
  /** 工作流/集合节点数量摘要 */
  node_count?: number;
  /** 更新类消息的变更摘要 */
  change_summary?: string;
  /** 本轮工具产出的可视化载荷，历史回放时按 kind 渲染 */
  views?: AIViewPayload[];
  created_at: string;
}

// AISessionScope 标识会话工作范围：
//  - 'environment'：环境级，Agent 可看到环境下所有配置（默认）
//  - 'config'：单配置范围，强绑 ConfigID（旧行为）
export type AISessionScope = 'general' | 'environment' | 'config';

// AISession 是一次完整的 AI 对话上下文。
export interface AISession {
  id: string;
  title: string;
  environment_id: string;
  /** 工作范围；旧会话由后端在加载时按 config_id 推断。 */
  scope: AISessionScope;
  /** 仅 scope='config' 时必填；环境级会话可以为空。 */
  config_id: string;
  context_prefetched: boolean;
  /** 会话级 artifact 编辑模式（Claude Code 式迭代上下文） */
  active_artifact_type?: 'assemble' | 'workflow' | '';
  active_artifact_id?: string;
  active_artifact_name?: string;
  /** 确认模式下等待用户应用的修改草案。 */
  pending_draft?: AIPendingDraft | null;
  messages: AISessionMessage[];
  created_at: string;
  updated_at: string;
}

// AIPendingDraft 是等待确认的 AI 修改草案（对应后端 core.AIPendingDraft）。
export interface AIPendingDraft {
  artifact_type: string;
  artifact_id: string;
  artifact_name: string;
  /** 落地校验后的完整资产 JSON（前端只透传，不解析）。 */
  draft_json: string;
  base_hash: string;
  change_summary: string;
  created_at: string;
}

// AIAssistantRequest 是统一 AI 助手请求。
export interface AIAssistantRequest {
  /** 单次请求 ID，用于匹配流式事件。 */
  request_id: string;
  /** 已存在的会话 ID。 */
  session_id: string;
  /** 兼容字段；传 auto 时由后端根据输入判断行为。 */
  operation?:
    | 'auto'
    | 'create_assemble'
    | 'update_assemble'
    | 'update_workflow'
    | 'fix_execution';
  /** 用户输入内容。 */
  message: string;
  /** target_select 后用户选定的本轮目标配置。 */
  target_config_id?: string;
  /** 当前正在迭代的资产类型。 */
  artifact_type?: 'assemble' | 'workflow';
  /** 当前正在迭代的资产 ID。 */
  artifact_id?: string;
  /** 要修复的失败执行 ID，operation=fix_execution 时必填。 */
  execution_id?: string;
}

// AITargetOption 是后端要求用户选择目标配置时返回的候选项。
export interface AITargetOption {
  id: string;
  name: string;
}

// AIAssistantEvent 是后端推送的 AI 助手事件。
export interface AIAssistantEvent {
  request_id: string;
  session_id?: string;
  type:
    | 'delta'
    | 'progress'
    | 'heartbeat'
    | 'workflow'
    | 'assemble'
    | 'doc'
    | 'target_select'
    | 'workflow_pending'
    | 'view'
    | 'done'
    | 'error';
  text?: string;
  view?: AIViewPayload;
  workflow_id?: string;
  workflow_name?: string;
  assemble_id?: string;
  assemble_name?: string;
  artifact_type?: 'assemble' | 'workflow' | 'doc';
  action_type?: 'create' | 'update';
  doc_id?: string;
  doc_title?: string;
  node_count?: number;
  change_summary?: string;
  target_options?: AITargetOption[];
}
