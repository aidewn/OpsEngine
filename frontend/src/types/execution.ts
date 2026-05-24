// 执行记录相关类型，与后端 core.ExecutionRecord 对齐

import type { WorkflowDef, NodeInstance, EdgeConfig } from './workflow';
import type { AssembleDef } from './assemble';

// 工作流执行状态
export type WorkflowStatus =
  | 'Idle'
  | 'Running'
  | 'Success'
  | 'Failed'
  | 'Terminated';

// 节点执行状态
export type NodeState =
  | 'Idle'
  | 'Configuring'
  | 'Checking'
  | 'CheckFailed'
  | 'Ready'
  | 'Executing'
  | 'Success'
  | 'Failed'
  | 'Skipped';

// 日志条目
export interface LogEntry {
  time: string; // ISO 字符串
  level: 'info' | 'warn' | 'error';
  message: string;
}

// 执行启动时打的工作流+集合不可变快照
export interface ExecutionSnapshot {
  workflow: WorkflowDef;
  assembles: Record<string, AssembleDef>;
}

// 完整执行记录
export interface ExecutionRecord {
  id: string;
  workflow_id: string;
  snapshot: ExecutionSnapshot;
  status: WorkflowStatus;
  started_at: string;
  finished_at?: string;
  node_states: Record<string, NodeState>;
  node_logs: Record<string, LogEntry[]>;
  variables: Record<string, unknown>;
  error?: string;
}

// 列表精简结构
export interface ExecutionSummary {
  id: string;
  workflow_id: string;
  workflow_name: string;
  status: WorkflowStatus;
  started_at: string;
  finished_at?: string;
  error?: string;
}

// 便利：从节点快照提取 nodes / edges（执行详情页画布渲染用）
export function snapshotGraph(snapshot: ExecutionSnapshot): {
  nodes: NodeInstance[];
  edges: EdgeConfig[];
} {
  return {
    nodes: snapshot.workflow.nodes,
    edges: snapshot.workflow.edges,
  };
}
