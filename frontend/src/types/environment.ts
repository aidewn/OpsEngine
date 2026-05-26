// 环境配置相关类型，与后端 core.EnvironmentDef 对齐
// 字段命名遵循后端 JSON tag（snake_case）

// 配置类型枚举：Phase 1 仅 SSH 真正可用，其它 kind 留壳位
export type EnvConfigKind = 'ssh' | 'docker' | 'k8s' | 'jenkins';

// 环境内单条配置项；fields 内容随 kind 不同而不同
export interface EnvConfigItem {
  id: string;
  name: string;
  kind: EnvConfigKind;
  description: string;
  fields: Record<string, unknown>;
}

// 业务/项目环境
export interface EnvironmentDef {
  id: string;
  name: string;
  description: string;
  configs: EnvConfigItem[];
}
