// 统一提取错误文案。
// Wails 绑定调用失败时 Promise 以「字符串」reject（不是 Error 实例），
// 若只判断 err instanceof Error 会丢掉真实后端错误、退化成通用文案——这里一并兼容。
export function errorMessage(err: unknown, fallback = '操作失败'): string {
  if (typeof err === 'string') return err || fallback;
  if (err instanceof Error) return err.message || fallback;
  if (err && typeof err === 'object' && 'message' in err) {
    const m = (err as { message?: unknown }).message;
    if (typeof m === 'string' && m) return m;
  }
  return fallback;
}
