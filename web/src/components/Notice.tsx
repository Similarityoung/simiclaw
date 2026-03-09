import { cn } from '../lib/ui';

interface NoticeProps {
  title: string;
  body: string;
  tone?: 'neutral' | 'danger';
  className?: string;
}

export default function Notice({ title, body, tone = 'neutral', className }: NoticeProps) {
  const cardClassName = cn(
    'ui-panel rounded-[20px] px-4 py-4',
    tone === 'danger'
      ? 'border-rose-400/25 bg-[linear-gradient(180deg,rgba(44,18,25,0.9),rgba(26,10,16,0.96))]'
      : 'bg-[linear-gradient(180deg,rgba(18,22,31,0.86),rgba(12,15,21,0.94))]',
    className,
  );

  return (
    <div className={cardClassName}>
      <div className="text-[14px] font-semibold tracking-[-0.012em] text-[var(--color-ink-strong)]">{title}</div>
      <div className="mt-1.5 text-[12px] leading-6 tracking-[-0.01em] text-[var(--color-ink-soft)]">{body}</div>
    </div>
  );
}
