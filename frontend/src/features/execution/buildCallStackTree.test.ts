// 调用栈树构建单元测试
// 使用 Node 内置 test runner（node:test）+ node:assert/strict
// 运行：从 frontend/ 目录执行
//   node --import tsx --test src/features/execution/buildCallStackTree.test.ts
// 项目尚未配置专门的 test 命令，但本文件类型完整、断言独立，
// 一旦引入 vitest/node:test runner 即可直接执行。

import { describe, it } from 'node:test';
import assert from 'node:assert/strict';

import { buildCallStackTree, summarizeFrameStatus } from './buildCallStackTree';
import type { ExecutionSnapshot, FrameState } from '@/types/execution';
import type { AssembleDef } from '@/types/assemble';
import type { WorkflowDef } from '@/types/workflow';

function makeWorkflow(): WorkflowDef {
  return {
    id: 'wf-1',
    name: '主流程',
    description: '',
    variables: [],
    nodes: [
      {
        instance_id: 'caller-1',
        type_id: 'assemble:asm-1',
        config: {},
        position: { x: 0, y: 0 },
      },
      {
        instance_id: 'node-a',
        type_id: 'print',
        config: {},
        position: { x: 0, y: 0 },
      },
    ],
    edges: [],
  };
}

function makeAssemble(id: string, name: string): AssembleDef {
  return {
    id,
    name,
    description: '',
    params: [],
    returns: [],
    variables: [],
    nodes: [
      {
        instance_id: 'inner-1',
        type_id: 'print',
        config: {},
        position: { x: 0, y: 0 },
      },
      {
        instance_id: 'inner-caller',
        type_id: 'assemble:asm-2',
        config: {},
        position: { x: 0, y: 0 },
      },
    ],
    edges: [],
  };
}

function makeNestedAssemble(): AssembleDef {
  return {
    id: 'asm-2',
    name: '嵌套集合',
    description: '',
    params: [],
    returns: [],
    variables: [],
    nodes: [
      {
        instance_id: 'deep-1',
        type_id: 'end',
        config: {},
        position: { x: 0, y: 0 },
      },
    ],
    edges: [],
  };
}

function snapshotWithAssembles(): ExecutionSnapshot {
  return {
    workflow: makeWorkflow(),
    assembles: {
      'asm-1': makeAssemble('asm-1', '集合 A'),
      'asm-2': makeNestedAssemble(),
    },
  };
}

describe('summarizeFrameStatus', () => {
  it('returns undefined when no node states recorded', () => {
    const frame: FrameState = {
      assemble_id: '',
      node_states: {},
      node_logs: {},
      variables: {},
    };
    assert.equal(summarizeFrameStatus(frame), undefined);
  });

  it('returns Failed if any node failed', () => {
    const frame: FrameState = {
      assemble_id: '',
      node_states: { a: 'Success', b: 'Failed' },
      node_logs: {},
      variables: {},
    };
    assert.equal(summarizeFrameStatus(frame), 'Failed');
  });

  it('returns Running if any node executing and no failure', () => {
    const frame: FrameState = {
      assemble_id: '',
      node_states: { a: 'Success', b: 'Executing' },
      node_logs: {},
      variables: {},
    };
    assert.equal(summarizeFrameStatus(frame), 'Running');
  });

  it('returns Success when all done', () => {
    const frame: FrameState = {
      assemble_id: '',
      node_states: { a: 'Success', b: 'Skipped' },
      node_logs: {},
      variables: {},
    };
    assert.equal(summarizeFrameStatus(frame), 'Success');
  });

  it('returns Terminated when has terminated but no failure / running', () => {
    const frame: FrameState = {
      assemble_id: '',
      node_states: { a: 'Success', b: 'Terminated' },
      node_logs: {},
      variables: {},
    };
    assert.equal(summarizeFrameStatus(frame), 'Terminated');
  });
});

describe('buildCallStackTree', () => {
  it('builds a single root when no children frames', () => {
    const rootFrame: FrameState = {
      assemble_id: '',
      node_states: { 'node-a': 'Success' },
      node_logs: {},
      variables: {},
    };
    const tree = buildCallStackTree(rootFrame, snapshotWithAssembles());

    assert.equal(tree.kind, 'root');
    assert.equal(tree.label, '主流程');
    assert.deepEqual(tree.framePath, []);
    assert.equal(tree.status, 'Success');
    assert.equal(tree.children.length, 0);
    assert.equal(tree.nodeEntries.length, 1);
    assert.equal(tree.nodeEntries[0]!.nodeId, 'node-a');
  });

  it('builds two-level tree (root + assemble child)', () => {
    const rootFrame: FrameState = {
      assemble_id: '',
      node_states: { 'caller-1': 'Executing', 'node-a': 'Success' },
      node_logs: {},
      variables: {},
      children: {
        'caller-1': {
          assemble_id: 'asm-1',
          node_states: { 'inner-1': 'Success' },
          node_logs: {},
          variables: {},
        },
      },
    };

    const tree = buildCallStackTree(rootFrame, snapshotWithAssembles());

    // root
    assert.equal(tree.children.length, 1);
    assert.equal(tree.status, 'Running'); // caller-1 still executing
    // child
    const child = tree.children[0]!;
    assert.equal(child.kind, 'assemble');
    assert.deepEqual(child.framePath, ['caller-1']);
    assert.equal(child.assembleId, 'asm-1');
    assert.equal(child.callerNodeId, 'caller-1');
    assert.ok(child.label.startsWith('集合 A'));
    assert.equal(child.status, 'Success');
    assert.equal(child.children.length, 0);
    assert.equal(child.nodeEntries.length, 1);
    assert.equal(child.nodeEntries[0]!.nodeId, 'inner-1');
  });

  it('recurses into nested assemble frames', () => {
    const rootFrame: FrameState = {
      assemble_id: '',
      node_states: {},
      node_logs: {},
      variables: {},
      children: {
        'caller-1': {
          assemble_id: 'asm-1',
          node_states: {},
          node_logs: {},
          variables: {},
          children: {
            'inner-caller': {
              assemble_id: 'asm-2',
              node_states: { 'deep-1': 'Success' },
              node_logs: {},
              variables: {},
            },
          },
        },
      },
    };

    const tree = buildCallStackTree(rootFrame, snapshotWithAssembles());
    const child = tree.children[0]!;
    assert.equal(child.children.length, 1);
    const grand = child.children[0]!;
    assert.deepEqual(grand.framePath, ['caller-1', 'inner-caller']);
    assert.equal(grand.assembleId, 'asm-2');
    assert.equal(grand.label.startsWith('嵌套集合'), true);
  });

  it('uses nodeTypeNames map for entry labels when available', () => {
    const rootFrame: FrameState = {
      assemble_id: '',
      node_states: { 'node-a': 'Success' },
      node_logs: {},
      variables: {},
    };
    const names = new Map<string, string>([['print', '打印日志']]);
    const tree = buildCallStackTree(rootFrame, snapshotWithAssembles(), names);
    assert.equal(tree.nodeEntries[0]!.label, '打印日志');
  });
});
