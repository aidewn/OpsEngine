// 通用 React 错误边界，避免单个组件 throw 导致整页白屏
// 用法：
//   <ErrorBoundary fallback={<div>出错了</div>}>...</ErrorBoundary>
//   或不传 fallback 使用默认 UI
//
// 注意：只能捕获渲染期 / 生命周期 / 构造期错误；不能捕获事件回调、setTimeout
// 等异步代码里的错误（按 React 文档惯例）。

import { Component, type ErrorInfo, type ReactNode } from 'react';

interface Props {
  children: ReactNode;
  // 自定义 fallback；接收错误对象以便定制展示
  fallback?: ReactNode | ((error: Error, reset: () => void) => ReactNode);
  // 错误上报钩子
  onError?: (error: Error, info: ErrorInfo) => void;
}

interface State {
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // 默认打到 console，便于 DevTools 排查
    console.error('[ErrorBoundary]', error, info);
    this.props.onError?.(error, info);
  }

  reset = () => this.setState({ error: null });

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;

    const { fallback } = this.props;
    if (typeof fallback === 'function') return fallback(error, this.reset);
    if (fallback !== undefined) return fallback;

    return (
      <div className="m-4 rounded border border-red-200 bg-red-50 p-4 text-sm text-red-700">
        <div className="mb-2 font-medium">页面渲染出错</div>
        <div className="mb-3 break-all font-mono text-xs">{error.message}</div>
        <button
          type="button"
          onClick={this.reset}
          className="rounded bg-red-600 px-3 py-1 text-xs font-medium text-white hover:bg-red-700"
        >
          重试
        </button>
      </div>
    );
  }
}
