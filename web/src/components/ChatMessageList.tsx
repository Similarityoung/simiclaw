import type { RefObject } from 'react';
import type { ChatMessageItem } from '../types';
import { formatDateTime } from '../lib/format';
import Notice from './Notice';
import styles from './ChatMessageList.module.css';

interface ChatMessageListProps {
  messages: ChatMessageItem[];
  historyCursor?: string;
  historyLoading: boolean;
  historyLoadingMore: boolean;
  historyError?: string;
  empty: boolean;
  onLoadMore: () => void;
  scrollRef: RefObject<HTMLDivElement>;
}

export default function ChatMessageList({
  messages,
  historyCursor,
  historyLoading,
  historyLoadingMore,
  historyError,
  empty,
  onLoadMore,
  scrollRef,
}: ChatMessageListProps) {
  return (
    <div className={styles.scrollArea} ref={scrollRef}>
      {historyCursor ? (
        <div className={styles.historyLoadRow}>
          <button className={styles.ghostButton} type="button" onClick={onLoadMore} disabled={historyLoadingMore}>
            {historyLoadingMore ? '加载更早消息…' : '加载更早消息'}
          </button>
        </div>
      ) : null}
      {historyLoading ? <Notice title="历史同步中" body="正在装载当前会话历史。" /> : null}
      {historyError ? <Notice title="历史加载失败" body={historyError} tone="danger" /> : null}
      {empty && !historyLoading ? (
        <div className={styles.emptyStage}>
          <div className={styles.emptyEyebrow}>新会话已就绪</div>
          <div className={styles.emptyTitle}>从一条消息开始这次对话</div>
          <div className={styles.emptyBody}>当前会话默认使用 DM 通道，调试流会在右侧实时展示状态、reasoning 和工具结果。</div>
        </div>
      ) : null}
      {messages.map((message) => (
        <article key={message.id} className={message.role === 'user' ? styles.userRow : styles.assistantRow}>
          <div className={styles.metaRow}>
            <span className={styles.role}>{message.role === 'user' ? 'You' : 'SimiClaw'}</span>
            <span className={styles.time}>{formatDateTime(message.createdAt)}</span>
          </div>
          <div className={message.role === 'user' ? styles.userBubble : styles.assistantBubble}>
            <div className={styles.content}>{message.content || ' '}</div>
            {message.streaming ? <span className={styles.streamingCursor} aria-hidden="true" /> : null}
          </div>
        </article>
      ))}
    </div>
  );
}
