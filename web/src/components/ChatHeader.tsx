import { formatRelativeTime } from '../lib/format';
import styles from './ChatHeader.module.css';

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
    <header className={styles.header}>
      <div className={styles.left}>
        <button className={styles.chromeButton} type="button" onClick={onToggleSidebar}>
          会话
        </button>
        <div>
          <div className={styles.title}>{conversation}</div>
          <div className={styles.subtitle}>
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
      <div className={styles.right}>
        <span className={styles.modelBadge}>{model}</span>
        <button className={styles.chromeButton} type="button" onClick={onToggleDebug}>
          调试流
        </button>
      </div>
    </header>
  );
}
