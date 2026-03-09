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
    <div className="ui-panel rounded-[24px] p-4 sm:p-5">
      <div className="flex flex-col gap-3 border-b border-white/8 pb-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="ui-kicker mb-2">Compose</div>
          <div className="text-[20px] font-semibold tracking-[-0.02em] text-[var(--color-ink-strong)]">消息输入</div>
          <div className="mt-1 text-[13px] text-[var(--color-ink-soft)]">
            conversation_id：<span>{conversationLabel}</span>
          </div>
        </div>
        <div className="text-[12px] uppercase tracking-[0.18em] text-[var(--color-ink-muted)]">
          {sending ? '流式处理中' : 'Enter 发送，Shift+Enter 换行'}
        </div>
      </div>
      <div className="ui-input-shell mt-4 overflow-hidden">
        <textarea
          className="ui-textarea"
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
      <div className="mt-4 flex flex-col gap-3 border-t border-white/8 pt-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-h-6 text-[13px] tracking-[-0.011em] text-[var(--color-ink-soft)]">
          {composeError ? <span className="text-rose-300">{composeError}</span> : <span>{statusLabel}</span>}
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <button className="ui-button-secondary" type="button" onClick={onOpenNewSession} disabled={sending}>
            切到新会话
          </button>
          <button className="ui-button-primary" type="button" onClick={onSend} disabled={sending || !composerText.trim()}>
            {sending ? '发送中…' : '发送消息'}
          </button>
        </div>
      </div>
    </div>
  );
}
