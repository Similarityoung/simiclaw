import type { SessionRecord } from '../types';
import { formatConversationLabel, formatCount, formatRelativeTime } from '../lib/format';
import Notice from './Notice';
import styles from './SessionSidebar.module.css';

interface SessionSidebarProps {
  sessions: SessionRecord[];
  filteredSessions: SessionRecord[];
  activeSessionKey: string;
  searchText: string;
  sessionsCursor?: string;
  sessionsLoading: boolean;
  sessionsBusy: boolean;
  sessionsError?: string;
  sending: boolean;
  open: boolean;
  onOpenNewSession: () => void;
  onRefresh: () => void;
  onSearchChange: (value: string) => void;
  onOpenSession: (session: SessionRecord) => void;
  onLoadMore: () => void;
}

export default function SessionSidebar({
  sessions,
  filteredSessions,
  activeSessionKey,
  searchText,
  sessionsCursor,
  sessionsLoading,
  sessionsBusy,
  sessionsError,
  sending,
  open,
  onOpenNewSession,
  onRefresh,
  onSearchChange,
  onOpenSession,
  onLoadMore,
}: SessionSidebarProps) {
  return (
    <aside className={open ? `${styles.sidebar} ${styles.open}` : styles.sidebar}>
      <div className={styles.inner}>
        <header className={styles.brandBlock}>
          <div className={styles.brandMark}>SC</div>
          <div>
            <div className={styles.brandTitle}>SimiClaw Web</div>
            <div className={styles.brandSubtitle}>agent runtime console</div>
          </div>
        </header>

        <div className={styles.actionsRow}>
          <button className={styles.primaryButton} type="button" onClick={onOpenNewSession} disabled={sending}>
            新建会话
          </button>
          <button className={styles.ghostButton} type="button" onClick={onRefresh} disabled={sessionsBusy || sending}>
            {sessionsBusy ? '刷新中…' : '刷新'}
          </button>
        </div>

        <label className={styles.searchField}>
          <span className={styles.searchLabel}>会话检索</span>
          <input
            value={searchText}
            onChange={(event) => onSearchChange(event.target.value)}
            placeholder="搜索 conversation / session / model"
          />
        </label>

        <div className={styles.metaRow}>
          <span>{formatCount(sessions.length)} 个会话</span>
          {sessionsCursor ? <span>支持加载更多</span> : <span>已到末尾</span>}
        </div>

        <div className={styles.sessionList}>
          {sessionsLoading ? <Notice title="会话装载中" body="正在读取最近活跃会话。" /> : null}
          {sessionsError ? <Notice title="会话列表异常" body={sessionsError} tone="danger" /> : null}
          {!sessionsLoading && filteredSessions.length === 0 ? (
            <Notice title="暂无匹配会话" body="可以新建一个会话，或者调整检索词。" />
          ) : null}
          {filteredSessions.map((session) => {
            const selected = session.session_key === activeSessionKey;
            return (
              <button
                key={session.session_key}
                type="button"
                className={selected ? `${styles.sessionCard} ${styles.sessionCardActive}` : styles.sessionCard}
                onClick={() => onOpenSession(session)}
                disabled={sending}
              >
                <div className={styles.sessionCardTop}>
                  <span className={styles.sessionCardTitle}>{formatConversationLabel(session.conversation_id)}</span>
                  <span className={styles.sessionCardTime}>{formatRelativeTime(session.last_activity_at)}</span>
                </div>
                <div className={styles.sessionCardSubline}>
                  <span>{session.last_model || 'model · unknown'}</span>
                  <span>{formatCount(session.message_count)} 条消息</span>
                </div>
                <div className={styles.sessionCardKey}>{session.session_key}</div>
              </button>
            );
          })}
        </div>

        {sessionsCursor ? (
          <button className={styles.loadMoreButton} type="button" onClick={onLoadMore} disabled={sessionsBusy || sending}>
            {sessionsBusy ? '加载中…' : '加载更多会话'}
          </button>
        ) : null}
      </div>
    </aside>
  );
}
