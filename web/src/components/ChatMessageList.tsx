import type { RefObject } from 'react';
import type { ChatMessageItem } from '../types';
import { formatDateTime } from '../lib/format';
import { cn } from '../lib/ui';
import Notice from './Notice';

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
    <div className="ui-scrollbar flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto pr-2" ref={scrollRef}>
      {historyCursor ? (
        <div className="flex justify-center pb-2">
          <button className="ui-button-secondary" type="button" onClick={onLoadMore} disabled={historyLoadingMore}>
            {historyLoadingMore ? '加载更早消息…' : '加载更早消息'}
          </button>
        </div>
      ) : null}
      {historyLoading ? <Notice title="历史同步中" body="正在装载当前会话历史。" /> : null}
      {historyError ? <Notice title="历史加载失败" body={historyError} tone="danger" /> : null}
      {empty && !historyLoading ? (
        <div className="ui-panel flex flex-col items-start gap-4 rounded-[28px] p-6 sm:p-8">
          <div className="ui-badge">新会话已就绪</div>
          <div className="text-[clamp(1.8rem,4vw,2.75rem)] font-medium leading-tight tracking-[-0.04em] text-[var(--color-ink-strong)]">
            从一条消息开始这次对话
          </div>
          <div className="max-w-2xl text-[15px] leading-7 tracking-[-0.011em] text-[var(--color-ink-soft)]">
            当前会话默认使用 DM 通道，调试流会在右侧实时展示状态、reasoning 和工具结果。
          </div>
        </div>
      ) : null}
      {messages.map((message) => (
        <article
          key={message.id}
          className={cn('flex flex-col gap-2', message.role === 'user' ? 'items-end' : 'items-start')}
        >
          <div
            className={cn(
              'flex w-full max-w-[90%] items-center gap-2 px-1 text-[12px] tracking-[0.08em] text-[var(--color-ink-muted)] sm:max-w-[82%]',
              message.role === 'user' ? 'justify-end text-right' : 'justify-start',
            )}
          >
            <span className="uppercase">{message.role === 'user' ? 'You' : 'SimiClaw'}</span>
            <span>{formatDateTime(message.createdAt)}</span>
          </div>
          <div
            className={cn(
              'relative w-full max-w-[90%] overflow-hidden rounded-[26px] border px-5 py-4 shadow-[0_20px_50px_rgba(3,6,18,0.18)] sm:max-w-[82%]',
              message.role === 'user'
                ? 'border-[rgba(124,147,255,0.22)] bg-[linear-gradient(180deg,rgba(124,147,255,0.22),rgba(69,87,196,0.18))] text-white'
                : 'border-white/10 bg-[linear-gradient(180deg,rgba(255,255,255,0.06),rgba(255,255,255,0.03))] text-[var(--color-ink)]',
            )}
          >
            <div className="whitespace-pre-wrap break-words text-[15px] leading-7 tracking-[-0.011em]">{message.content || ' '}</div>
            {message.streaming ? (
              <span className="ml-1 inline-block h-5 w-2 animate-pulse rounded-full bg-[var(--color-accent)] align-middle" aria-hidden="true" />
            ) : null}
          </div>
        </article>
      ))}
    </div>
  );
}
