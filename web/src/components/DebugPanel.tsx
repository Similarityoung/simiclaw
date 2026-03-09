import type { RefObject } from 'react';
import type { DebugEntry } from '../types';
import { formatCount } from '../lib/format';
import { cn } from '../lib/ui';
import Notice from './Notice';

interface DebugPanelProps {
  debugEntries: DebugEntry[];
  sessionKey?: string;
  open: boolean;
  persistent: boolean;
  scrollRef: RefObject<HTMLDivElement>;
  onToggle: () => void;
}

export default function DebugPanel({ debugEntries, sessionKey, open, persistent, scrollRef, onToggle }: DebugPanelProps) {
  return (
    <aside
      hidden={!open && !persistent}
      aria-hidden={!open && !persistent}
      className={cn(
        'ui-panel-strong fixed inset-y-4 right-4 z-20 flex w-[min(24rem,calc(100vw-2rem))] flex-col p-4 transition-transform duration-300 ease-out xl:sticky xl:top-0 xl:right-auto xl:inset-y-auto xl:h-[calc(100vh-3rem)] xl:w-auto xl:min-w-[21rem] xl:max-w-[24rem]',
        open ? 'translate-x-0' : 'translate-x-[110%] xl:translate-x-0',
      )}
    >
      <div className="flex items-start justify-between gap-3 border-b border-white/8 pb-4">
        <div>
          <div className="ui-kicker mb-2">Runtime trace</div>
          <div className="text-[20px] font-semibold tracking-[-0.02em] text-[var(--color-ink-strong)]">运行流</div>
          <div className="mt-1 text-[13px] tracking-[-0.011em] text-[var(--color-ink-soft)]">status / reasoning / tools / terminal</div>
        </div>
        <button className="ui-button-chrome" type="button" onClick={onToggle}>
          收起
        </button>
      </div>
      <div className="mt-4 flex items-center justify-between gap-3 text-[12px] tracking-[-0.01em] text-[var(--color-ink-muted)]">
        <span>{formatCount(debugEntries.length)} 个流事件</span>
        <span>{sessionKey ? `session=${sessionKey}` : 'draft session'}</span>
      </div>
      <div className="ui-scrollbar mt-4 flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto pr-1" ref={scrollRef}>
        {debugEntries.length === 0 ? (
          <Notice title="等待流事件" body="发送消息后，这里会依次显示 accepted、status、reasoning、tool 和 terminal。" />
        ) : null}
        {debugEntries.map((entry) => (
          <article key={entry.id} className={cardClassName(entry)}>
            <div className="flex items-start justify-between gap-3">
              <span className="text-[14px] font-semibold tracking-[-0.012em] text-[var(--color-ink-strong)]">{entry.title}</span>
              <span className="shrink-0 text-[11px] uppercase tracking-[0.14em] text-[var(--color-ink-muted)]">{entry.at}</span>
            </div>
            {entry.body ? <div className="mt-2 whitespace-pre-wrap text-[13px] leading-6 tracking-[-0.01em] text-[var(--color-ink-soft)]">{entry.body}</div> : null}
            {entry.payload ? (
              <pre className="ui-scrollbar mt-3 max-h-72 overflow-auto rounded-[18px] border border-white/8 bg-black/20 px-4 py-3 text-[12px] leading-6 tracking-[-0.01em] text-[var(--color-ink-soft)]">
                {entry.payload}
              </pre>
            ) : null}
          </article>
        ))}
      </div>
    </aside>
  );
}

function cardClassName(entry: DebugEntry): string {
  switch (entry.tone) {
    case 'success':
      return 'rounded-[22px] border border-emerald-300/20 bg-[linear-gradient(180deg,rgba(25,52,47,0.64),rgba(14,30,26,0.9))] px-4 py-4 shadow-[0_14px_30px_rgba(5,18,14,0.22)]';
    case 'danger':
      return 'rounded-[22px] border border-rose-300/20 bg-[linear-gradient(180deg,rgba(56,22,31,0.7),rgba(28,11,18,0.92))] px-4 py-4 shadow-[0_14px_30px_rgba(20,6,10,0.2)]';
    case 'warning':
      return 'rounded-[22px] border border-amber-300/20 bg-[linear-gradient(180deg,rgba(56,41,18,0.7),rgba(28,18,10,0.92))] px-4 py-4 shadow-[0_14px_30px_rgba(20,14,6,0.2)]';
    default:
      return 'rounded-[22px] border border-white/8 bg-[linear-gradient(180deg,rgba(255,255,255,0.06),rgba(255,255,255,0.03))] px-4 py-4 shadow-[0_16px_32px_rgba(3,6,18,0.16)]';
  }
}
