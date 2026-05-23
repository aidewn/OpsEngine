// 全屏居中消息（加载中/错误提示）

export function CenteredMessage({
  children,
  tone = 'info',
}: {
  children: React.ReactNode;
  tone?: 'info' | 'error';
}) {
  return (
    <div className="flex h-screen items-center justify-center">
      <div
        className={
          tone === 'error'
            ? 'text-sm text-red-600'
            : 'text-sm text-slate-500'
        }
      >
        {children}
      </div>
    </div>
  );
}
