import styles from './Composer.module.css';

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
    <div className={styles.dock}>
      <div className={styles.header}>
        <div>
          <div className={styles.title}>消息输入</div>
          <div className={styles.subline}>
            conversation_id：<span>{conversationLabel}</span>
          </div>
        </div>
        <div className={styles.subline}>{sending ? '流式处理中' : 'Enter 发送，Shift+Enter 换行'}</div>
      </div>
      <textarea
        className={styles.input}
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
      <div className={styles.footer}>
        <div className={styles.inlineStatus}>
          {composeError ? <span className={styles.statusDanger}>{composeError}</span> : <span>{statusLabel}</span>}
        </div>
        <div className={styles.actions}>
          <button className={styles.ghostButton} type="button" onClick={onOpenNewSession} disabled={sending}>
            切到新会话
          </button>
          <button className={styles.primaryButton} type="button" onClick={onSend} disabled={sending || !composerText.trim()}>
            {sending ? '发送中…' : '发送消息'}
          </button>
        </div>
      </div>
    </div>
  );
}
