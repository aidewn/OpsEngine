// 环境配置相关类型，与后端 core.EnvironmentDef 对齐
// 字段命名遵循后端 JSON tag（snake_case）

// 配置类型枚举：
//   ssh/docker/k8s/jenkins —— 远端目标
//   localhost —— 直连本机（无字段）
//   registry  —— 镜像仓库（image_push_tar 等节点引用）
export type EnvConfigKind =
  | 'ssh'
  | 'docker'
  | 'k8s'
  | 'jenkins'
  | 'localhost'
  | 'registry';

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
