// ActionCard 组件：统一渲染 AI 消息中的可执行动作卡片。
import { Button } from '@/components/ui/Button';
import { cn } from '@/lib/cn';

interface ActionCardProps {
  title: string;
  description?: string;
  tone?: 'info' | 'success' | 'warning';
  primaryAction?: {
    label: string;
    loading?: boolean;
    disabled?: boolean;
    onClick: () => void;
  };
  secondaryAction?: {
    label: string;
    disabled?: boolean;
    onClick: () => void;
  };
  tertiaryAction?: {
    label: string;
    disabled?: boolean;
    onClick: () => void;
  };
}

const toneClass = {
  info: 'border-ops-info bg-ops-info-soft',
  success: 'border-ops-success bg-ops-success-soft',
  warning: 'border-ops-warning bg-ops-warning-soft',
};

export function ActionCard({
  title,
  description,
  tone = 'info',
  primaryAction,
  secondaryAction,
  tertiaryAction,
}: ActionCardProps) {
  return (
    <div className={cn('mt-3 rounded-md border px-3 py-2', toneClass[tone])}>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="truncate text-sm font-medium text-ops-primary">{title}</div>
          {description ? <div className="mt-1 text-xs text-ops-secondary">{description}</div> : null}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {tertiaryAction ? (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              disabled={tertiaryAction.disabled}
              onClick={tertiaryAction.onClick}
            >
              {tertiaryAction.label}
            </Button>
          ) : null}
          {secondaryAction ? (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              disabled={secondaryAction.disabled}
              onClick={secondaryAction.onClick}
            >
              {secondaryAction.label}
            </Button>
          ) : null}
          {primaryAction ? (
            <Button
              type="button"
              size="sm"
              disabled={primaryAction.disabled || primaryAction.loading}
              onClick={primaryAction.onClick}
            >
              {primaryAction.loading ? '处理中…' : primaryAction.label}
            </Button>
          ) : null}
        </div>
      </div>
    </div>
  );
}
