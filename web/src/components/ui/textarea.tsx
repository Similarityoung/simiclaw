import * as React from 'react';

import { cn } from '../../lib/ui';

const Textarea = React.forwardRef<HTMLTextAreaElement, React.ComponentProps<'textarea'>>(({ className, ...props }, ref) => {
  return (
    <textarea
      ref={ref}
      className={cn(
        'flex min-h-[104px] w-full rounded-[20px] border border-white/10 bg-[linear-gradient(180deg,rgba(255,255,255,0.06),rgba(255,255,255,0.03))] px-5 py-4 text-[15px] leading-7 tracking-[-0.011em] text-[var(--color-ink)] shadow-[inset_0_1px_0_rgba(255,255,255,0.04),0_18px_40px_rgba(3,6,18,0.14)] transition-all duration-150 ease-out placeholder:text-[color:var(--color-ink-muted)] focus-visible:border-[rgba(124,147,255,0.54)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[rgba(124,147,255,0.16)] disabled:cursor-not-allowed disabled:opacity-50',
        'flex min-h-[104px] w-full rounded-[20px] border border-[rgba(15,23,42,0.08)] bg-white/88 px-5 py-4 text-[15px] leading-7 tracking-[-0.011em] text-[var(--color-ink)] shadow-[0_10px_24px_rgba(148,163,184,0.12)] transition-all duration-150 ease-out placeholder:text-[color:var(--color-ink-muted)] focus-visible:border-[rgba(59,130,246,0.32)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[rgba(59,130,246,0.12)] disabled:cursor-not-allowed disabled:opacity-50',
        className,
      )}
      {...props}
    />
  );
});

Textarea.displayName = 'Textarea';

export { Textarea };
