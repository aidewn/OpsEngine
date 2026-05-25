// 节点复制 / 剪切 / 粘贴的快捷键处理 hook
// 复用于工作流编辑页和集合编辑页
//
// 关键设计：
//   - selectedNodeIds / graph / onGraphChange / clipboard 都用 useRef 缓存最新值
//   - useEffect 只依赖 editorKey（稳定字符串），keydown 监听只注册一次
//   - 避免父组件 re-render 时 hook 反复 register/unregister 监听导致卡死
//
// 行为：
//   - 复制：过滤掉内部单例节点（system_* / assemble_*）+ 框选内连线一并带上
//   - 粘贴：新 ID 重新生成，放到鼠标当前位置（最左上节点为锚点）
//   - 跨编辑器粘贴：sourceEditorKey 不匹配 → 清空剪贴板

import { useEffect, useRef } from 'react';
import { useReactFlow } from '@xyflow/react';
import { useClipboard, type ClipboardData } from './ClipboardContext';
import { isInternalNodeType } from '@/types/nodeType';
import { newUUID } from '@/lib/uuid';
import type { EdgeConfig, NodeInstance } from '@/types/workflow';
import type { GraphDef } from '../workflow/canvasMapping';

interface Options<T extends GraphDef> {
  editorKey: string; // "workflow:<id>" 或 "assemble:<id>"
  graph: T;
  selectedNodeIds: Set<string>;
  onGraphChange: (next: T) => void;
}

export function useCopyPaste<T extends GraphDef>({
  editorKey,
  graph,
  selectedNodeIds,
  onGraphChange,
}: Options<T>) {
  const clipboard = useClipboard();
  const rf = useReactFlow();

  // 用 ref 缓存最新值，避免 useEffect 依赖在每次 render 时变化
  const graphRef = useRef(graph);
  graphRef.current = graph;
  const selectedRef = useRef(selectedNodeIds);
  selectedRef.current = selectedNodeIds;
  const onChangeRef = useRef(onGraphChange);
  onChangeRef.current = onGraphChange;
  const clipboardRef = useRef(clipboard);
  clipboardRef.current = clipboard;
  const rfRef = useRef(rf);
  rfRef.current = rf;

  // 维护最后一次鼠标位置（用于粘贴）
  const lastMousePos = useRef({ x: 0, y: 0 });
  useEffect(() => {
    const move = (e: MouseEvent) => {
      lastMousePos.current = { x: e.clientX, y: e.clientY };
    };
    window.addEventListener('mousemove', move);
    return () => window.removeEventListener('mousemove', move);
  }, []);

  // 监听快捷键：仅依赖 editorKey 注册一次，避免父组件 re-render 反复 register
  useEffect(() => {
    // 未加载完的编辑器短路（editorKey 形如 "workflow:none"）
    if (editorKey.endsWith(':none')) return;

    const handler = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey;
      if (!mod) return;
      // 输入框/文本域内不拦截
      const target = e.target as HTMLElement;
      if (
        target &&
        (target.tagName === 'INPUT' ||
          target.tagName === 'TEXTAREA' ||
          target.isContentEditable)
      ) {
        return;
      }

      const k = e.key.toLowerCase();
      if (k === 'c') {
        if (selectedRef.current.size === 0) return;
        e.preventDefault();
        doCopy(editorKey, graphRef.current, selectedRef.current, clipboardRef.current.set);
      } else if (k === 'x') {
        if (selectedRef.current.size === 0) return;
        e.preventDefault();
        const next = doCut(
          editorKey,
          graphRef.current,
          selectedRef.current,
          clipboardRef.current.set,
        );
        if (next) onChangeRef.current(next as T);
      } else if (k === 'v') {
        if (!clipboardRef.current.data) return;
        e.preventDefault();
        if (clipboardRef.current.data.sourceEditorKey !== editorKey) {
          // 跨 tab 粘贴：清空
          clipboardRef.current.clear();
          return;
        }
        const flowPos = rfRef.current.screenToFlowPosition(lastMousePos.current);
        const next = doPaste(graphRef.current, clipboardRef.current.data, flowPos);
        onChangeRef.current(next as T);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [editorKey]);
}

// 复制（不修改 graph）
function doCopy(
  editorKey: string,
  graph: GraphDef,
  selected: Set<string>,
  setClipboard: (d: ClipboardData) => void,
) {
  const { copyable, innerEdges } = collectCopyable(graph, selected);
  if (copyable.length === 0) return;
  setClipboard({
    sourceEditorKey: editorKey,
    nodes: copyable,
    edges: innerEdges,
  });
}

// 剪切 = 复制 + 删除可删除节点
function doCut<T extends GraphDef>(
  editorKey: string,
  graph: T,
  selected: Set<string>,
  setClipboard: (d: ClipboardData) => void,
): T | null {
  const { copyable, innerEdges } = collectCopyable(graph, selected);
  if (copyable.length === 0) return null;
  setClipboard({
    sourceEditorKey: editorKey,
    nodes: copyable,
    edges: innerEdges,
  });
  // 删除可剪切的节点 + 它们关联的所有边
  const toDelete = new Set(copyable.map((n) => n.instance_id));
  return {
    ...graph,
    nodes: graph.nodes.filter((n) => !toDelete.has(n.instance_id)),
    edges: graph.edges.filter(
      (e) => !toDelete.has(e.from.node) && !toDelete.has(e.to.node),
    ),
  } as T;
}

// 粘贴：在鼠标位置创建新节点（最左上为锚点）
function doPaste<T extends GraphDef>(
  graph: T,
  data: ClipboardData,
  flowPos: { x: number; y: number },
): T {
  if (data.nodes.length === 0) return graph;

  // 计算原节点最左上坐标作为锚点
  const minX = Math.min(...data.nodes.map((n) => n.position.x));
  const minY = Math.min(...data.nodes.map((n) => n.position.y));

  // 重新生成 instance_id 并记录映射
  const idMap = new Map<string, string>();
  const newNodes: NodeInstance[] = data.nodes.map((n) => {
    const newID = newUUID();
    idMap.set(n.instance_id, newID);
    return {
      ...n,
      instance_id: newID,
      position: {
        x: n.position.x - minX + flowPos.x,
        y: n.position.y - minY + flowPos.y,
      },
    };
  });
  const newEdges: EdgeConfig[] = data.edges.map((e) => ({
    from: { node: idMap.get(e.from.node) ?? e.from.node, port: e.from.port },
    to: { node: idMap.get(e.to.node) ?? e.to.node, port: e.to.port },
  }));

  return {
    ...graph,
    nodes: [...graph.nodes, ...newNodes],
    edges: [...graph.edges, ...newEdges],
  } as T;
}

// 收集可复制的节点 + 它们之间的内部连线
function collectCopyable(graph: GraphDef, selected: Set<string>): {
  copyable: NodeInstance[];
  innerEdges: EdgeConfig[];
} {
  const copyable = graph.nodes.filter(
    (n) => selected.has(n.instance_id) && !isInternalNodeType(n.type_id),
  );
  const copyableIDs = new Set(copyable.map((n) => n.instance_id));
  const innerEdges = graph.edges.filter(
    (e) => copyableIDs.has(e.from.node) && copyableIDs.has(e.to.node),
  );
  return { copyable, innerEdges };
}
