// 浏览器原生 UUID 生成，无需额外依赖
// 兼容性：现代浏览器都支持 crypto.randomUUID()
export function newUUID(): string {
  return crypto.randomUUID();
}
