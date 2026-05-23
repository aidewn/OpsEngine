// 拖拽数据格式约定：从左侧栏拖到画布时携带的节点元信息
// 通过 dataTransfer 在 DOM 拖拽事件间传递

export const DRAG_MIME = 'application/x-opsengine-node';

// 拖拽 payload：目标节点类型 + 预填 config
export interface DragNodePayload {
  type_id: string;
  config: Record<string, unknown>;
}

export function setDragPayload(
  e: React.DragEvent | DragEvent,
  payload: DragNodePayload,
) {
  e.dataTransfer?.setData(DRAG_MIME, JSON.stringify(payload));
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'copy';
  }
}

export function readDragPayload(e: DragEvent): DragNodePayload | null {
  const raw = e.dataTransfer?.getData(DRAG_MIME);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as DragNodePayload;
  } catch {
    return null;
  }
}
