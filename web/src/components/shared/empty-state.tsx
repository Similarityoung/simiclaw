import type { LucideIcon } from 'lucide-react';
import { Inbox } from 'lucide-react';

import { cn } from '@/lib/utils';

interface EmptyStateProps {
  title: string;
  body: string;
  eyebrow?: string;
  icon?: LucideIcon;
  className?: string;
}

export function EmptyState({ title, body, eyebrow = 'No Data', icon: Icon = Inbox, className }: EmptyStateProps): JSX.Element {
  return (
    <div className={cn('surface-card grid-noise flex flex-col gap-3 p-6', className)}>
      <div className="flex items-center gap-2 text-xs uppercase tracking-[0.28em] text-muted-foreground">
        <Icon className="h-4 w-4" />
        {eyebrow}
      </div>
      <div>
        <div className="text-lg font-semibold">{title}</div>
        <p className="mt-1 text-sm leading-6 text-muted-foreground">{body}</p>
      </div>
    </div>
  );
}
