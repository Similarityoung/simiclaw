import type { RefObject } from 'react';
import type { DebugEntry } from '../types';
import { formatCount } from '../lib/format';
import Notice from './Notice';
import styles from './DebugPanel.module.css';

interface DebugPanelProps {
  debugEntries: DebugEntry[];
  sessionKey?: string;
  open: boolean;
  scrollRef: RefObject<HTMLDivElement>;
  onToggle: () => void;
}

export default function DebugPanel({ debugEntries, sessionKey, open, scrollRef, onToggle }: DebugPanelProps) {
  return (
    <aside className={open ? `${styles.panel} ${styles.open}` : styles.panel}>
      <div className={styles.header}>
        <div>
          <div className={styles.title}>运行流</div>
          <div className={styles.subtitle}>status / reasoning / tools / terminal</div>
        </div>
        <button className={styles.chromeButton} type="button" onClick={onToggle}>
          收起
        </button>
      </div>
      <div className={styles.statsRow}>
        <span>{formatCount(debugEntries.length)} 个流事件</span>
        <span>{sessionKey ? `session=${sessionKey}` : 'draft session'}</span>
      </div>
      <div className={styles.viewport} ref={scrollRef}>
        {debugEntries.length === 0 ? (
          <Notice title="等待流事件" body="发送消息后，这里会依次显示 accepted、status、reasoning、tool 和 terminal。" />
        ) : null}
        {debugEntries.map((entry) => (
          <article key={entry.id} className={cardClassName(entry, styles)}>
            <div className={styles.cardHeader}>
              <span className={styles.cardTitle}>{entry.title}</span>
              <span className={styles.cardTime}>{entry.at}</span>
            </div>
            {entry.body ? <div className={styles.cardBody}>{entry.body}</div> : null}
            {entry.payload ? <pre className={styles.cardPayload}>{entry.payload}</pre> : null}
          </article>
        ))}
      </div>
    </aside>
  );
}

function cardClassName(entry: DebugEntry, sheet: Record<string, string>): string {
  switch (entry.tone) {
    case 'success':
      return `${sheet.card} ${sheet.cardSuccess}`;
    case 'danger':
      return `${sheet.card} ${sheet.cardDanger}`;
    case 'warning':
      return `${sheet.card} ${sheet.cardWarning}`;
    default:
      return sheet.card;
  }
}
