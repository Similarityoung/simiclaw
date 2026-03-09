import { formatRelativeTime } from '../lib/format';

interface ChatHeaderProps {
  conversation: string;
  status?: string;
  lastActivity?: string;
  model: string;
  onToggleSidebar: () => void;
  onToggleDebug: () => void;
}

export default function ChatHeader({
  conversation,
  status,
  lastActivity,
  model,
  onToggleSidebar,
  onToggleDebug,
}: ChatHeaderProps) {
  const relativeLastActivity = formatRelativeTime(lastActivity);

  return (
    <header className="flex flex-wrap items-start justify-between gap-4 border-b border-white/10 pb-6">
      <div className="flex min-w-0 items-start gap-3">
        <button className="ui-button-chrome md:hidden" type="button" onClick={onToggleSidebar}>
          会话
        </button>
        <div className="min-w-0">
          <div className="ui-kicker mb-2">Live workspace</div>
          <div className="truncate text-[clamp(1.75rem,3vw,2.5rem)] font-medium tracking-[-0.03em] text-[var(--color-ink-strong)]">
            {conversation}
          </div>
          <div className="mt-2 flex flex-wrap items-center gap-2 text-[13px] tracking-[-0.011em] text-[var(--color-ink-soft)]">
            <span>{status || '等待输入'}</span>
            {relativeLastActivity !== '—' ? (
              <>
                <span>·</span>
                <span>{relativeLastActivity}</span>
              </>
            ) : null}
          </div>
        </div>
      </div>
      <div className="flex items-center gap-3 self-start">
        <span className="ui-badge max-w-[220px] truncate">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-300 shadow-[0_0_14px_rgba(110,231,183,0.55)]" aria-hidden="true" />
          {model}
        </span>
        <button className="ui-button-chrome" type="button" onClick={onToggleDebug}>
          调试流
        </button>
      </div>
    </header>
  );
}
