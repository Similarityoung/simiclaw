import { AlertTriangle } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface ErrorStateProps {
  title?: string;
  body: string;
  actionLabel?: string;
  onAction?: () => void;
  className?: string;
}

export function ErrorState({
  title = '数据暂时不可用',
  body,
  actionLabel = '重试',
  onAction,
  className,
}: ErrorStateProps): JSX.Element {
  return (
    <div className={cn('surface-card flex flex-col gap-4 p-6', className)}>
      <div className="flex items-center gap-2 text-xs uppercase tracking-[0.28em] text-muted-foreground">
        <AlertTriangle className="h-4 w-4" />
        Error
      </div>
      <div>
        <div className="text-lg font-semibold">{title}</div>
        <p className="mt-1 text-sm leading-6 text-muted-foreground">{body}</p>
      </div>
      {onAction ? (
        <div>
          <Button variant="outline" onClick={onAction}>
            {actionLabel}
          </Button>
        </div>
      ) : null}
    </div>
  );
}
