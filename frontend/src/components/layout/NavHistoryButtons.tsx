// 路由历史按钮：提供标题栏内的后退和前进入口。
import { useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { ArrowLeft, ArrowRight } from 'lucide-react';

export function NavHistoryButtons() {
  const navigate = useNavigate();
  const canGoBack = useMemo(() => window.history.length > 1, []);

  return (
    <div className="titlebar-no-drag flex items-center gap-1">
      <button
        type="button"
        className="titlebar-icon-button"
        disabled={!canGoBack}
        aria-label="后退"
        title="后退"
        onClick={() => navigate(-1)}
      >
        <ArrowLeft size={14} />
      </button>
      <button
        type="button"
        className="titlebar-icon-button"
        aria-label="前进"
        title="前进"
        onClick={() => navigate(1)}
      >
        <ArrowRight size={14} />
      </button>
    </div>
  );
}
