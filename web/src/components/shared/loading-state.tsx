import { LoaderCircle } from 'lucide-react';

import { cn } from '@/lib/utils';

interface LoadingStateProps {
  title?: string;
  body?: string;
  className?: string;
}

export function LoadingState({
  title = '正在同步数据',
  body = '控制台正在读取当前工作区的最新状态。',
  className,
}: LoadingStateProps): JSX.Element {
  return (
    <div className={cn('surface-card flex flex-col gap-3 p-6', className)}>
      <div className="flex items-center gap-3 text-xs uppercase tracking-[0.28em] text-muted-foreground">
        <LoaderCircle className="h-4 w-4 animate-spin" />
        Loading
      </div>
      <div>
        <div className="text-lg font-semibold">{title}</div>
        <p className="mt-1 text-sm text-muted-foreground">{body}</p>
      </div>
    </div>
  );
}
