import * as React from 'react';
import { Slot } from '@radix-ui/react-slot';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '../../lib/ui';

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-[18px] text-[14px] font-medium tracking-[-0.012em] transition-all duration-150 ease-out select-none disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[rgba(59,130,246,0.2)] focus-visible:ring-offset-0 active:scale-[0.985]',
  {
    variants: {
      variant: {
        default:
          'min-h-11 border border-[rgba(59,130,246,0.12)] bg-[linear-gradient(180deg,#7fb2ff_0%,#5d9cff_55%,#3b82f6_100%)] px-5 text-white shadow-[0_10px_24px_rgba(59,130,246,0.22)] hover:-translate-y-0.5 hover:brightness-105',
        secondary:
          'min-h-11 border border-[rgba(15,23,42,0.08)] bg-white/84 px-5 text-[var(--color-ink)] shadow-[0_8px_24px_rgba(148,163,184,0.14)] hover:-translate-y-0.5 hover:border-[rgba(15,23,42,0.12)] hover:bg-white',
        chrome:
          'min-h-10 rounded-full border border-[rgba(15,23,42,0.08)] bg-white/78 px-4 text-[13px] text-[var(--color-ink-soft)] shadow-[0_8px_18px_rgba(148,163,184,0.12)] hover:border-[rgba(15,23,42,0.12)] hover:bg-white hover:text-[var(--color-ink-strong)]',
        ghost:
          'min-h-10 px-3 text-[var(--color-ink-soft)] hover:bg-[rgba(241,245,249,0.9)] hover:text-[var(--color-ink-strong)]',
      },
      size: {
        default: '',
        sm: 'min-h-9 rounded-[14px] px-3 text-[13px]',
        icon: 'size-10 rounded-full px-0',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : 'button';
    return <Comp className={cn(buttonVariants({ variant, size }), className)} ref={ref} {...props} />;
  },
);

Button.displayName = 'Button';

export { Button, buttonVariants };
