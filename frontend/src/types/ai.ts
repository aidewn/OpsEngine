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
  /** 隐藏消息（预取的服务器状态等），前端不渲染。 */
  hidden?: boolean;
  created_at: string;
}

// AISession 是一次完整的 AI 对话上下文。
export interface AISession {
  id: string;
  title: string;
  environment_id: string;
  config_id: string;
  context_prefetched: boolean;
  messages: AISessionMessage[];
  created_at: string;
  updated_at: string;
}

// AIAssistantRequest 是统一 AI 助手请求。
export interface AIAssistantRequest {
  /** 单次请求 ID，用于匹配流式事件。 */
  request_id: string;
  /** 已存在的会话 ID。 */
  session_id: string;
  /** 兼容字段；传 auto 时由后端根据输入判断行为。 */
  operation?: 'auto';
  /** 用户输入内容。 */
  message: string;
}

// AIAssistantEvent 是后端推送的 AI 助手事件。
export interface AIAssistantEvent {
  request_id: string;
  session_id?: string;
  type: 'delta' | 'progress' | 'workflow' | 'done' | 'error';
  text?: string;
  workflow_id?: string;
  workflow_name?: string;
}
