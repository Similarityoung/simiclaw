import { motion } from 'framer-motion';
import { Plus, RefreshCw, Search, Sparkles } from 'lucide-react';

import type { SessionRecord } from '../types';
import { formatConversationLabel, formatCount, formatRelativeTime } from '../lib/format';
import { cn } from '../lib/ui';
import Notice from './Notice';
import { Button } from './ui/button';
import { Input } from './ui/input';

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
  className?: string;
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
  className,
  onOpenNewSession,
  onRefresh,
  onSearchChange,
  onOpenSession,
  onLoadMore,
}: SessionSidebarProps) {
  return (
    <motion.aside
      initial={{ x: -24, opacity: 0 }}
      animate={{ x: 0, opacity: 1 }}
      transition={{ duration: 0.18, ease: 'easeOut' }}
      className={cn(
        'ui-panel-strong flex h-full min-h-0 flex-col overflow-hidden p-3',
        className,
      )}
    >
      <div className="flex h-full flex-col gap-4">
        <header className="ui-panel flex items-center gap-3 rounded-[22px] px-4 py-4">
          <div className="flex h-12 w-12 items-center justify-center rounded-[16px] border border-[rgba(15,23,42,0.08)] bg-[linear-gradient(180deg,rgba(255,255,255,0.98),rgba(219,234,254,0.8))] text-[15px] font-semibold tracking-[0.18em] text-[var(--color-accent-strong)] shadow-[0_10px_30px_rgba(59,130,246,0.12)]">
            <Sparkles className="h-5 w-5" />
          </div>
          <div>
            <div className="text-[18px] font-semibold tracking-[-0.02em] text-[var(--color-ink-strong)]">SimiClaw Web</div>
            <div className="mt-1 text-[12px] uppercase tracking-[0.18em] text-[var(--color-ink-muted)]">agent runtime console</div>
          </div>
        </header>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-1 xl:grid-cols-2">
          <Button className="w-full" type="button" onClick={onOpenNewSession} disabled={sending}>
            <Plus className="h-4 w-4" />
            新建会话
          </Button>
          <Button className="w-full" variant="secondary" type="button" onClick={onRefresh} disabled={sessionsBusy || sending}>
            <RefreshCw className={cn('h-4 w-4', sessionsBusy && 'animate-spin')} />
            {sessionsBusy ? '刷新中…' : '刷新'}
          </Button>
        </div>

        <label className="block">
          <span className="ui-kicker mb-2 block">会话检索</span>
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--color-ink-muted)]" />
            <Input
              className="pl-10"
              value={searchText}
              onChange={(event) => onSearchChange(event.target.value)}
              placeholder="搜索 conversation / session / model"
            />
          </div>
        </label>

        <div className="flex items-center justify-between gap-3 text-[12px] tracking-[-0.01em] text-[var(--color-ink-muted)]">
          <span>{formatCount(sessions.length)} 个会话</span>
          <span>{sessionsCursor ? '支持加载更多' : '已到末尾'}</span>
        </div>

        <div className="ui-scrollbar flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto pr-1">
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
                className={cn(
                  'group rounded-[22px] border px-4 py-4 text-left transition-all duration-200 ease-out',
                  'bg-[linear-gradient(180deg,rgba(255,255,255,0.98),rgba(248,250,252,0.94))] shadow-[0_12px_24px_rgba(148,163,184,0.12)] hover:-translate-y-0.5',
                  selected
                    ? 'border-[rgba(59,130,246,0.24)] bg-[linear-gradient(180deg,rgba(219,234,254,0.95),rgba(255,255,255,0.98))] shadow-[0_14px_28px_rgba(59,130,246,0.12)]'
                    : 'border-[rgba(15,23,42,0.08)] hover:border-[rgba(15,23,42,0.12)] hover:bg-[linear-gradient(180deg,rgba(255,255,255,1),rgba(248,250,252,0.96))]',
                )}
                onClick={() => onOpenSession(session)}
                disabled={sending}
              >
                <div className="flex items-start justify-between gap-3">
                  <span className="line-clamp-2 text-[15px] font-semibold tracking-[-0.015em] text-[var(--color-ink-strong)]">
                    {formatConversationLabel(session.conversation_id)}
                  </span>
                  <span className="shrink-0 text-[11px] uppercase tracking-[0.14em] text-[var(--color-ink-muted)]">
                    {formatRelativeTime(session.last_activity_at)}
                  </span>
                </div>
                <div className="mt-3 flex items-center justify-between gap-3 text-[12px] tracking-[-0.01em] text-[var(--color-ink-soft)]">
                  <span>{session.last_model || 'model · unknown'}</span>
                  <span>{formatCount(session.message_count)} 条消息</span>
                </div>
                <div className="mt-3 truncate text-[11px] tracking-[0.08em] text-[var(--color-ink-muted)]">
                  {session.session_key}
                </div>
              </button>
            );
          })}
        </div>

        {sessionsCursor ? (
          <Button className="w-full" variant="secondary" type="button" onClick={onLoadMore} disabled={sessionsBusy || sending}>
            {sessionsBusy ? '加载中…' : '加载更多会话'}
          </Button>
        ) : null}
      </div>
    </motion.aside>
  );
}
