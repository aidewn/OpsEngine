// 编辑态探测相关类型（与后端 main.ProbeEnvNodeRequest/Result 对齐）
// 字段命名遵循后端 JSON tag（snake_case）

export interface ProbeItem {
  key: string;
  label: string;
  meta?: Record<string, unknown>;
}

export interface ProbeEnvNodeRequest {
  type_id: string;
  env_id: string;
  config_id: string;
  node_config: Record<string, unknown>;
}

export interface ProbeEnvNodeResult {
  items: ProbeItem[];
}

// probe_snapshot 持久化形态（写入节点 config.probe_snapshot）
export interface ProbeSnapshot {
  picked_key: string;
  picked_label: string;
  items: ProbeItem[];
  captured_at: string; // ISO8601
}

// variable_bindings：勾选「同步到工作流变量」时使用，可多条
export interface VariableBinding {
  variable_name: string;
  output_port: string;
}
