// hasWailsRuntime 判断当前页面是否运行在 Wails WebView 中。
export function hasWailsRuntime() {
  return Boolean((window as Window & { runtime?: unknown }).runtime);
}
