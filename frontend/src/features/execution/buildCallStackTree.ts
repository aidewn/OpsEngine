// 调用栈树构建：把 ExecutionRecord.root_frame 递归转成 UI 友好的 CallStackNode
// 与 ExecutionStore 的 FrameState 一一对应；纯函数，不依赖 React

import type {
  ExecutionSnapshot,
  FrameState,
  NodeState,
  WorkflowStatus,
} from '@/types/execution';
import type { NodeInstance } from '@/types/workflow';

// 调用栈节点（侧栏渲染单元）
export interface CallStackNode {
  // 定位 frame，[] = 主流
  framePath: string[];
  // 显示名（主流为 workflow.name；集合为 assemble.name）
  label: string;
  kind: 'root' | 'assemble';
  // 父图上的调用节点 instance_id（assemble 帧才有）
  callerNodeId?: string;
  // 解析后的 assemble id（assemble 帧才有）
  assembleId?: string;
  // frame 级状态汇总，根据其下 node_states 推导
  status?: WorkflowStatus;
  children: CallStackNode[];
  // 当前 frame 内已经报状态的节点摘要（可选展开使用）
  nodeEntries: CallStackNodeEntry[];
}

export interface CallStackNodeEntry {
  nodeId: string;
  state: NodeState;
  label: string;
}

// 根据 frame 内所有节点状态汇总成 frame 级 WorkflowStatus
// 规则（按优先级）：
//   1. 任一节点 Failed / CheckFailed → Failed
//   2. 任一节点 Executing → Running
//   3. 全部 Success / Skipped → Success
//   4. 有 Terminated 且无上述 → Terminated
//   5. 无节点状态 → undefined
export function summarizeFrameStatus(
  frame: FrameState,
): WorkflowStatus | undefined {
  const states = Object.values(frame.node_states ?? {});
  if (states.length === 0) return undefined;

  let hasExecuting = false;
  let hasTerminated = false;
  let allDone = true;

  for (const s of states) {
    if (s === 'Failed' || s === 'CheckFailed') return 'Failed';
    if (s === 'Executing' || s === 'Checking' || s === 'Configuring' || s === 'Ready') {
      hasExecuting = true;
      allDone = false;
    } else if (s === 'Terminated') {
      hasTerminated = true;
      allDone = false;
    } else if (s !== 'Success' && s !== 'Skipped') {
      // Idle 等也算未完成
      allDone = false;
    }
  }

  if (hasExecuting) return 'Running';
  if (allDone) return 'Success';
  if (hasTerminated) return 'Terminated';
  return undefined;
}

// 在给定节点集合中找 instance_id 对应节点（O(n)，frame 体量小）
function findNodeInstance(
  nodes: NodeInstance[],
  instanceId: string,
): NodeInstance | undefined {
  return nodes.find((n) => n.instance_id === instanceId);
}

// 根据 frame.node_states 构造节点条目列表
function buildNodeEntries(
  frame: FrameState,
  graphNodes: NodeInstance[],
  nodeTypeNames?: Map<string, string>,
): CallStackNodeEntry[] {
  const entries: CallStackNodeEntry[] = [];
  for (const [nodeId, state] of Object.entries(frame.node_states ?? {})) {
    const inst = findNodeInstance(graphNodes, nodeId);
    const label =
      (inst && nodeTypeNames?.get(inst.type_id)) ?? inst?.type_id ?? nodeId;
    entries.push({ nodeId, state, label });
  }
  return entries;
}

// 主入口：递归构造调用栈树
//
// 参数：
//   rootFrame      - ExecutionRecord.root_frame
//   snapshot       - ExecutionSnapshot（提供工作流/集合名 + 节点列表）
//   nodeTypeNames  - 可选，type_id → display_name 映射；缺省时回退到 type_id
export function buildCallStackTree(
  rootFrame: FrameState,
  snapshot: ExecutionSnapshot,
  nodeTypeNames?: Map<string, string>,
): CallStackNode {
  const wf = snapshot.workflow;

  const root: CallStackNode = {
    framePath: [],
    label: wf.name || '(unnamed workflow)',
    kind: 'root',
    status: summarizeFrameStatus(rootFrame),
    children: buildChildren(rootFrame, [], wf.nodes, snapshot, nodeTypeNames),
    nodeEntries: buildNodeEntries(rootFrame, wf.nodes, nodeTypeNames),
  };
  return root;
}

// 递归处理 frame.children；parentNodes 用于解析 callerNodeId 对应节点名
function buildChildren(
  parentFrame: FrameState,
  parentPath: string[],
  parentNodes: NodeInstance[],
  snapshot: ExecutionSnapshot,
  nodeTypeNames?: Map<string, string>,
): CallStackNode[] {
  const children = parentFrame.children;
  if (!children) return [];

  const out: CallStackNode[] = [];
  for (const [callerNodeId, childFrame] of Object.entries(children)) {
    const path = [...parentPath, callerNodeId];
    const assembleId = childFrame.assemble_id;
    const asm = assembleId ? snapshot.assembles[assembleId] : undefined;
    const asmName = asm?.name ?? assembleId ?? '(unknown assemble)';

    // 父图上的调用节点（用于将来在 label 后缀显示 via xxx，本期省略）
    const callerInstance = findNodeInstance(parentNodes, callerNodeId);
    const callerDisplay = callerInstance
      ? (nodeTypeNames?.get(callerInstance.type_id) ?? callerInstance.type_id)
      : undefined;

    const childNodes = asm?.nodes ?? [];

    const node: CallStackNode = {
      framePath: path,
      label: callerDisplay ? `${asmName} · ${callerDisplay}` : asmName,
      kind: 'assemble',
      callerNodeId,
      assembleId,
      status: summarizeFrameStatus(childFrame),
      children: buildChildren(
        childFrame,
        path,
        childNodes,
        snapshot,
        nodeTypeNames,
      ),
      nodeEntries: buildNodeEntries(childFrame, childNodes, nodeTypeNames),
    };
    out.push(node);
  }
  return out;
}
