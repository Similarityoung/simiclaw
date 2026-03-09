import { motion } from 'framer-motion';
import { ArrowUpRight, MessageSquarePlus } from 'lucide-react';

import { Button } from './ui/button';
import { Textarea } from './ui/textarea';

interface ComposerProps {
  conversationLabel: string;
  composerText: string;
  sending: boolean;
  composeError?: string;
  statusLabel?: string;
  onChange: (value: string) => void;
  onSend: () => void;
  onOpenNewSession: () => void;
}

export default function Composer({
  conversationLabel,
  composerText,
  sending,
  composeError,
  statusLabel,
  onChange,
  onSend,
  onOpenNewSession,
}: ComposerProps) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.2, ease: 'easeOut' }}
      className="ui-panel shrink-0 rounded-[24px] p-4"
    >
      <div className="flex flex-col gap-2.5 border-b border-[rgba(15,23,42,0.08)] pb-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="ui-kicker mb-2">Compose</div>
          <div className="text-[18px] font-semibold tracking-[-0.02em] text-[var(--color-ink-strong)]">消息输入</div>
          <div className="mt-1 text-[13px] text-[var(--color-ink-soft)]">
            conversation_id：<span>{conversationLabel}</span>
          </div>
        </div>
        <div className="text-[12px] uppercase tracking-[0.18em] text-[var(--color-ink-muted)]">
          {sending ? '流式处理中' : 'Enter 发送，Shift+Enter 换行'}
        </div>
      </div>
      <div className="mt-3 overflow-hidden rounded-[20px]">
        <Textarea
          value={composerText}
          onChange={(event) => onChange(event.target.value)}
          placeholder="输入消息，开始一次新的 agent run…"
          rows={4}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && !event.shiftKey) {
              event.preventDefault();
              onSend();
            }
          }}
        />
      </div>
      <div className="mt-3 flex flex-col gap-3 border-t border-[rgba(15,23,42,0.08)] pt-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-h-6 text-[13px] tracking-[-0.011em] text-[var(--color-ink-soft)]">
          {composeError ? <span className="text-rose-300">{composeError}</span> : <span>{statusLabel}</span>}
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <Button variant="secondary" type="button" onClick={onOpenNewSession} disabled={sending}>
            <MessageSquarePlus className="h-4 w-4" />
            切到新会话
          </Button>
          <Button type="button" onClick={onSend} disabled={sending || !composerText.trim()}>
            <ArrowUpRight className="h-4 w-4" />
            {sending ? '发送中…' : '发送消息'}
          </Button>
        </div>
      </div>
    </motion.div>
  );
}
