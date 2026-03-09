import type { SessionRecord } from '../types';
import { formatConversationLabel, formatCount, formatRelativeTime } from '../lib/format';
import { cn } from '../lib/ui';
import Notice from './Notice';

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
  persistent: boolean;
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
  persistent,
  onOpenNewSession,
  onRefresh,
  onSearchChange,
  onOpenSession,
  onLoadMore,
}: SessionSidebarProps) {
  return (
    <aside
      hidden={!open && !persistent}
      aria-hidden={!open && !persistent}
      className={cn(
        'ui-panel-strong fixed inset-y-4 left-4 z-30 w-[min(22rem,calc(100vw-2rem))] overflow-hidden p-3 transition-transform duration-300 ease-out md:sticky md:top-0 md:left-auto md:inset-y-auto md:z-auto md:block md:h-[calc(100vh-3rem)] md:w-auto md:min-w-[18rem] md:max-w-[22rem]',
        open ? 'translate-x-0' : '-translate-x-[110%] md:translate-x-0',
      )}
    >
      <div className="flex h-full flex-col gap-4">
        <header className="ui-panel flex items-center gap-3 rounded-[22px] px-4 py-4">
          <div className="flex h-12 w-12 items-center justify-center rounded-[16px] border border-white/14 bg-[linear-gradient(180deg,rgba(255,255,255,0.16),rgba(124,147,255,0.16))] text-[15px] font-semibold tracking-[0.18em] text-white shadow-[0_10px_30px_rgba(87,110,255,0.18)]">
            SC
          </div>
          <div>
            <div className="text-[18px] font-semibold tracking-[-0.02em] text-[var(--color-ink-strong)]">SimiClaw Web</div>
            <div className="mt-1 text-[12px] uppercase tracking-[0.18em] text-[var(--color-ink-muted)]">agent runtime console</div>
          </div>
        </header>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-1 xl:grid-cols-2">
          <button className="ui-button-primary w-full" type="button" onClick={onOpenNewSession} disabled={sending}>
            新建会话
          </button>
          <button className="ui-button-secondary w-full" type="button" onClick={onRefresh} disabled={sessionsBusy || sending}>
            {sessionsBusy ? '刷新中…' : '刷新'}
          </button>
        </div>

        <label className="block">
          <span className="ui-kicker mb-2 block">会话检索</span>
          <div className="ui-input-shell">
            <input
              className="ui-input"
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
                  'bg-[linear-gradient(180deg,rgba(255,255,255,0.06),rgba(255,255,255,0.03))] shadow-[inset_0_1px_0_rgba(255,255,255,0.04)] hover:-translate-y-0.5',
                  selected
                    ? 'border-[rgba(124,147,255,0.42)] bg-[linear-gradient(180deg,rgba(124,147,255,0.2),rgba(255,255,255,0.06))] shadow-[0_10px_28px_rgba(92,110,255,0.18)]'
                    : 'border-white/8 hover:border-white/14 hover:bg-[linear-gradient(180deg,rgba(255,255,255,0.08),rgba(255,255,255,0.04))]',
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
          <button className="ui-button-secondary w-full" type="button" onClick={onLoadMore} disabled={sessionsBusy || sending}>
            {sessionsBusy ? '加载中…' : '加载更多会话'}
          </button>
        ) : null}
      </div>
    </aside>
  );
}
