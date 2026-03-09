import { motion } from 'framer-motion';
import { PanelLeftOpen, Sparkles } from 'lucide-react';

import { formatRelativeTime } from '../lib/format';
import { Button } from './ui/button';

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
    <motion.header
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.18, ease: 'easeOut' }}
      className="flex flex-wrap items-start justify-between gap-4 border-b border-[rgba(15,23,42,0.08)] pb-5"
    >
      <div className="flex min-w-0 items-start gap-3">
        <Button className="xl:hidden" variant="chrome" type="button" onClick={onToggleSidebar}>
          <PanelLeftOpen className="h-4 w-4" />
          会话
        </Button>
        <div className="min-w-0">
          <div className="ui-kicker mb-2">Live workspace</div>
          <div className="truncate text-[clamp(1.5rem,2.4vw,2.15rem)] font-medium tracking-[-0.03em] text-[var(--color-ink-strong)]">
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
          <Sparkles className="h-3.5 w-3.5 text-emerald-300" aria-hidden="true" />
          {model}
        </span>
        <Button variant="chrome" type="button" onClick={onToggleDebug}>
          调试流
        </Button>
      </div>
    </motion.header>
  );
}
