import type { ReactNode } from 'react';

import { cn } from '@/lib/utils';

interface SectionHeaderProps {
  eyebrow: string;
  title: string;
  body?: string;
  action?: ReactNode;
  className?: string;
}

export function SectionHeader({ eyebrow, title, body, action, className }: SectionHeaderProps): JSX.Element {
  return (
    <div className={cn('flex flex-col gap-4 md:flex-row md:items-end md:justify-between', className)}>
      <div>
        <div className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{eyebrow}</div>
        <h1 className="mt-3 text-3xl font-semibold tracking-tight md:text-4xl">{title}</h1>
        {body ? <p className="mt-3 max-w-3xl text-sm leading-7 text-muted-foreground">{body}</p> : null}
      </div>
      {action ? <div className="shrink-0">{action}</div> : null}
    </div>
  );
}
