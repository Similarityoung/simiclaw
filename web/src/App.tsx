import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import styles from './App.module.css';
import runtimeClient from './lib/client';
import { APIError } from './lib/stream';
import {
  appendUserMessage,
  applyStreamEvent,
  createInitialRunState,
  mapHistoryMessage,
  type LiveRunState,
} from './lib/chatState';
import {
  formatConversationLabel,
  formatCount,
  formatDateTime,
  formatRelativeTime,
} from './lib/format';
import type {
  ChatMessageItem,
  DebugEntry,
  IngestRequest,
  RuntimeClient,
  SessionRecord,
} from './types';

const sessionPageSize = 30;
const historyPageSize = 40;

interface AppProps {
  client?: RuntimeClient;
  initialSessionKey?: string;
}

export default function App({ client = runtimeClient, initialSessionKey }: AppProps) {
  const [sessions, setSessions] = useState<SessionRecord[]>([]);
  const [sessionsCursor, setSessionsCursor] = useState<string | undefined>();
  const [sessionsLoading, setSessionsLoading] = useState(true);
  const [sessionsBusy, setSessionsBusy] = useState(false);
  const [sessionsError, setSessionsError] = useState<string>();
  const [searchText, setSearchText] = useState('');

  const [activeSessionKey, setActiveSessionKey] = useState(initialSessionKey ?? getSessionKeyFromURL());
  const [activeSessionMeta, setActiveSessionMeta] = useState<SessionRecord | null>(null);
  const [activeConversationID, setActiveConversationID] = useState('');
  const [historyCursor, setHistoryCursor] = useState<string | undefined>();
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyLoadingMore, setHistoryLoadingMore] = useState(false);
  const [historyError, setHistoryError] = useState<string>();

  const [runState, setRunState] = useState<LiveRunState>(() => createInitialRunState());
  const [composerText, setComposerText] = useState('');
  const [sending, setSending] = useState(false);
  const [composeError, setComposeError] = useState<string>();
  const [newSessionOpen, setNewSessionOpen] = useState(false);
  const [newConversationInput, setNewConversationInput] = useState('');
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [debugOpen, setDebugOpen] = useState(false);

  const activeSessionKeyRef = useRef(activeSessionKey);
  const activeConversationRef = useRef(activeConversationID);
  const runStateRef = useRef(runState);
  const historyRequestRef = useRef(0);
  const sequenceRef = useRef(Date.now());
  const sendAbortRef = useRef<AbortController | null>(null);
  const messagesViewportRef = useRef<HTMLDivElement | null>(null);
  const debugViewportRef = useRef<HTMLDivElement | null>(null);
  const messageScrollRestoreRef = useRef<{ top: number; height: number } | null>(null);

  const filteredSessions = useMemo(() => {
    const query = searchText.trim().toLowerCase();
    if (!query) {
      return sessions;
    }
    return sessions.filter((item) => {
      const conversation = item.conversation_id?.toLowerCase() ?? '';
      const model = item.last_model?.toLowerCase() ?? '';
      return (
        item.session_key.toLowerCase().includes(query) ||
        conversation.includes(query) ||
        model.includes(query)
      );
    });
  }, [searchText, sessions]);

  const currentConversationID = activeConversationID || activeSessionMeta?.conversation_id || '';
  const topbarStatus = sending ? runState.statusLabel || '处理中' : runState.errorText || runState.statusLabel;

  useEffect(() => {
    activeSessionKeyRef.current = activeSessionKey;
  }, [activeSessionKey]);

  useEffect(() => {
    activeConversationRef.current = activeConversationID;
  }, [activeConversationID]);

  useEffect(() => {
    runStateRef.current = runState;
  }, [runState]);

  useEffect(() => {
    syncSessionKeyToURL(activeSessionKey);
  }, [activeSessionKey]);

  useEffect(() => {
    const viewport = messagesViewportRef.current;
    if (!viewport || historyLoadingMore) {
      return;
    }
    const restore = messageScrollRestoreRef.current;
    if (restore) {
      messageScrollRestoreRef.current = null;
      window.requestAnimationFrame(() => {
        viewport.scrollTop = restore.top + (viewport.scrollHeight - restore.height);
      });
      return;
    }
    window.requestAnimationFrame(() => {
      viewport.scrollTop = viewport.scrollHeight;
    });
  }, [runState.messages, historyLoadingMore]);

  useEffect(() => {
    const viewport = debugViewportRef.current;
    if (!viewport) {
      return;
    }
    window.requestAnimationFrame(() => {
      viewport.scrollTop = viewport.scrollHeight;
    });
  }, [runState.debugEntries]);

  useEffect(() => {
    void bootstrap();
    return () => {
      sendAbortRef.current?.abort();
    };
  }, []);

  const bootstrap = useCallback(async () => {
    try {
      setSessionsLoading(true);
      setSessionsError(undefined);
      const page = await client.listSessions({ limit: sessionPageSize });
      setSessions(page.items);
      setSessionsCursor(page.next_cursor);

      const desiredSessionKey = initialSessionKey ?? getSessionKeyFromURL();
      if (desiredSessionKey) {
        const fromList = page.items.find((item) => item.session_key === desiredSessionKey);
        if (fromList) {
          await openSession(fromList);
          return;
        }
        try {
          const fetched = await client.getSession(desiredSessionKey);
          setSessions((current) => upsertSession(current, fetched));
          await openSession(fetched);
          return;
        } catch {
        }
      }

      if (page.items.length > 0) {
        await openSession(page.items[0]);
        return;
      }

      startDraftSession(generateDefaultConversationID());
    } catch (error) {
      setSessionsError(toErrorMessage(error));
      if (!activeConversationRef.current) {
        startDraftSession(generateDefaultConversationID());
      }
    } finally {
      setSessionsLoading(false);
    }
  }, [client, initialSessionKey]);

  const refreshSessions = useCallback(
    async (options?: { keepSelection?: boolean }) => {
      try {
        setSessionsBusy(true);
        setSessionsError(undefined);
        const page = await client.listSessions({ limit: sessionPageSize });
        setSessions(page.items);
        setSessionsCursor(page.next_cursor);
        if (options?.keepSelection !== false) {
          const selectedKey = activeSessionKeyRef.current || runStateRef.current.sessionKey;
          if (selectedKey) {
            const matched = page.items.find((item) => item.session_key === selectedKey);
            if (matched) {
              setActiveSessionKey(matched.session_key);
              setActiveSessionMeta(matched);
              setActiveConversationID(matched.conversation_id || activeConversationRef.current);
            }
          }
        }
      } catch (error) {
        setSessionsError(toErrorMessage(error));
      } finally {
        setSessionsBusy(false);
      }
    },
    [client],
  );

  const loadMoreSessions = useCallback(async () => {
    if (!sessionsCursor || sessionsBusy) {
      return;
    }
    try {
      setSessionsBusy(true);
      const page = await client.listSessions({ cursor: sessionsCursor, limit: sessionPageSize });
      setSessions((current) => mergeSessions(current, page.items));
      setSessionsCursor(page.next_cursor);
    } catch (error) {
      setSessionsError(toErrorMessage(error));
    } finally {
      setSessionsBusy(false);
    }
  }, [client, sessionsBusy, sessionsCursor]);

  const openSession = useCallback(
    async (session: SessionRecord) => {
      historyRequestRef.current += 1;
      const requestID = historyRequestRef.current;
      messageScrollRestoreRef.current = null;
      try {
        setHistoryLoading(true);
        setHistoryError(undefined);
        setComposeError(undefined);
        setActiveSessionKey(session.session_key);
        setActiveSessionMeta(session);
        setActiveConversationID(session.conversation_id || '');
        const page = await client.getSessionHistory({
          sessionKey: session.session_key,
          limit: historyPageSize,
          visibleOnly: true,
        });
        if (requestID !== historyRequestRef.current) {
          return;
        }
        const messages = page.items
          .map(mapHistoryMessage)
          .filter((item): item is ChatMessageItem => item !== null);
        setRunState(createInitialRunState(messages));
        setHistoryCursor(page.next_cursor);
        setDebugOpen(false);
        if (window.innerWidth < 960) {
          setSidebarOpen(false);
        }
      } catch (error) {
        if (requestID !== historyRequestRef.current) {
          return;
        }
        setHistoryError(toErrorMessage(error));
      } finally {
        if (requestID === historyRequestRef.current) {
          setHistoryLoading(false);
        }
      }
    },
    [client],
  );

  const loadMoreHistory = useCallback(async () => {
    if (!historyCursor || !activeSessionKeyRef.current || historyLoadingMore) {
      return;
    }
    const sessionKey = activeSessionKeyRef.current;
    const viewport = messagesViewportRef.current;
    messageScrollRestoreRef.current = viewport
      ? { top: viewport.scrollTop, height: viewport.scrollHeight }
      : null;
    try {
      setHistoryLoadingMore(true);
      const page = await client.getSessionHistory({
        sessionKey,
        cursor: historyCursor,
        limit: historyPageSize,
        visibleOnly: true,
      });
      if (sessionKey !== activeSessionKeyRef.current) {
        messageScrollRestoreRef.current = null;
        return;
      }
      const olderMessages = page.items
        .map(mapHistoryMessage)
        .filter((item): item is ChatMessageItem => item !== null);
      setRunState((current) => ({
        ...current,
        messages: [...olderMessages, ...current.messages],
      }));
      setHistoryCursor(page.next_cursor);
    } catch (error) {
      setHistoryError(toErrorMessage(error));
    } finally {
      setHistoryLoadingMore(false);
    }
  }, [client, historyCursor, historyLoadingMore]);

  const startDraftSession = useCallback((conversationID?: string) => {
    const nextConversation = conversationID?.trim() || generateDefaultConversationID();
    historyRequestRef.current += 1;
    messageScrollRestoreRef.current = null;
    setActiveSessionKey('');
    setActiveSessionMeta(null);
    setActiveConversationID(nextConversation);
    setHistoryCursor(undefined);
    setHistoryError(undefined);
    setComposeError(undefined);
    setRunState(createInitialRunState([]));
    setComposerText('');
    setNewSessionOpen(false);
    setNewConversationInput('');
    if (window.innerWidth < 960) {
      setSidebarOpen(false);
    }
  }, []);

  const handleCreateSession = useCallback(() => {
    startDraftSession(newConversationInput);
  }, [newConversationInput, startDraftSession]);

  const handleSend = useCallback(async () => {
    if (sending) {
      return;
    }
    const text = composerText.trim();
    if (!text) {
      return;
    }
    const conversationID = currentConversationID || generateDefaultConversationID();
    if (!currentConversationID) {
      setActiveConversationID(conversationID);
    }

    const request = buildIngestRequest(conversationID, sequenceRef.current, text, activeSessionMeta);
    sequenceRef.current += 1;

    sendAbortRef.current?.abort();
    const controller = new AbortController();
    sendAbortRef.current = controller;

    setComposerText('');
    setComposeError(undefined);
    setHistoryError(undefined);
    setSending(true);
    if (window.innerWidth < 1280) {
      setDebugOpen(true);
    }
    setRunState((current) => {
      const seeded = appendUserMessage(
        {
          ...current,
          debugEntries: [],
          assistantMessageID: undefined,
          errorText: undefined,
          statusLabel: '请求发送中',
        },
        text,
      );
      return {
        ...seeded,
        debugEntries: [],
        assistantMessageID: undefined,
      };
    });

    try {
      const record = await client.sendChat(request, {
        signal: controller.signal,
        onEvent: async (event) => {
          setRunState((current) => applyStreamEvent(current, event));
        },
      });

      if (record.session_key) {
        setActiveSessionKey(record.session_key);
      }
      await refreshSessions();
    } catch (error) {
      const message = toErrorMessage(error);
      setComposeError(message);
      setRunState((current) => ({
        ...current,
        statusLabel: '发送失败',
        errorText: message,
        debugEntries: [
          ...current.debugEntries,
          {
            id: `debug-client-error-${Date.now()}`,
            kind: 'error',
            title: '客户端异常',
            body: message,
            at: new Date().toISOString(),
            tone: 'danger',
          },
        ],
      }));
    } finally {
      sendAbortRef.current = null;
      setSending(false);
    }
  }, [activeSessionMeta, client, composerText, currentConversationID, refreshSessions, sending]);

  const activeSummary = useMemo(() => {
    const conversation = formatConversationLabel(currentConversationID);
    const model = activeSessionMeta?.last_model || (activeSessionMeta ? 'model · unknown' : 'draft');
    const lastActivity = activeSessionMeta?.last_activity_at || runState.messages[runState.messages.length - 1]?.createdAt;
    return {
      conversation,
      model,
      lastActivity,
    };
  }, [activeSessionMeta, currentConversationID, runState.messages]);

  return (
    <div className={styles.appShell}>
      <div className={styles.backdropGlow} aria-hidden="true" />
      <aside className={sidebarClassName(sidebarOpen)}>
        <div className={styles.sidebarInner}>
          <header className={styles.brandBlock}>
            <div className={styles.brandMark}>SC</div>
            <div>
              <div className={styles.brandTitle}>SimiClaw Web</div>
              <div className={styles.brandSubtitle}>agent runtime console</div>
            </div>
          </header>

          <div className={styles.sidebarActionsRow}>
            <button className={styles.primaryButton} type="button" onClick={() => setNewSessionOpen(true)} disabled={sending}>
              新建会话
            </button>
            <button className={styles.ghostButton} type="button" onClick={() => void refreshSessions()} disabled={sessionsBusy || sending}>
              {sessionsBusy ? '刷新中…' : '刷新'}
            </button>
          </div>

          <label className={styles.searchField}>
            <span className={styles.searchLabel}>会话检索</span>
            <input
              value={searchText}
              onChange={(event) => setSearchText(event.target.value)}
              placeholder="搜索 conversation / session / model"
            />
          </label>

          <div className={styles.sessionMetaRow}>
            <span>{formatCount(sessions.length)} 个会话</span>
            {sessionsCursor ? <span>支持加载更多</span> : <span>已到末尾</span>}
          </div>

          <div className={styles.sessionList}>
            {sessionsLoading ? <SidebarNotice title="会话装载中" body="正在读取最近活跃会话。" /> : null}
            {sessionsError ? <SidebarNotice title="会话列表异常" body={sessionsError} tone="danger" /> : null}
            {!sessionsLoading && filteredSessions.length === 0 ? (
              <SidebarNotice title="暂无匹配会话" body="可以新建一个会话，或者调整检索词。" />
            ) : null}
            {filteredSessions.map((session) => {
              const selected = session.session_key === activeSessionKey;
              return (
                <button
                  key={session.session_key}
                  type="button"
                  className={selected ? styles.sessionCardActive : styles.sessionCard}
                  onClick={() => void openSession(session)}
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
            <button className={styles.loadMoreButton} type="button" onClick={() => void loadMoreSessions()} disabled={sessionsBusy || sending}>
              {sessionsBusy ? '加载中…' : '加载更多会话'}
            </button>
          ) : null}
        </div>
      </aside>

      <main className={styles.mainColumn}>
        <header className={styles.mainHeader}>
          <div className={styles.mainHeaderLeft}>
            <button className={styles.chromeButton} type="button" onClick={() => setSidebarOpen((current) => !current)}>
              会话
            </button>
            <div>
              <div className={styles.pageTitle}>{activeSummary.conversation}</div>
              <div className={styles.pageSubtitle}>
                <span>{topbarStatus || '等待输入'}</span>
                <span>·</span>
                <span>{formatRelativeTime(activeSummary.lastActivity)}</span>
              </div>
            </div>
          </div>
          <div className={styles.mainHeaderRight}>
            <span className={styles.modelBadge}>{activeSummary.model}</span>
            <button className={styles.chromeButton} type="button" onClick={() => setDebugOpen((current) => !current)}>
              调试流
            </button>
          </div>
        </header>

        <section className={styles.chatStage}>
          <div className={styles.chatScrollArea} ref={messagesViewportRef}>
            {historyCursor ? (
              <div className={styles.historyLoadRow}>
                <button className={styles.ghostButton} type="button" onClick={() => void loadMoreHistory()} disabled={historyLoadingMore || sending}>
                  {historyLoadingMore ? '加载更早消息…' : '加载更早消息'}
                </button>
              </div>
            ) : null}
            {historyLoading ? <InlineNotice title="历史同步中" body="正在装载当前会话历史。" /> : null}
            {historyError ? <InlineNotice title="历史加载失败" body={historyError} tone="danger" /> : null}
            {runState.messages.length === 0 && !historyLoading ? (
              <div className={styles.emptyStage}>
                <div className={styles.emptyEyebrow}>新会话已就绪</div>
                <div className={styles.emptyTitle}>从一条消息开始这次对话</div>
                <div className={styles.emptyBody}>
                  当前会话默认使用 DM 通道，调试流会在右侧实时展示状态、reasoning 和工具结果。
                </div>
              </div>
            ) : null}
            {runState.messages.map((message) => (
              <article
                key={message.id}
                className={message.role === 'user' ? styles.userMessageRow : styles.assistantMessageRow}
              >
                <div className={styles.messageMetaRow}>
                  <span className={styles.messageRole}>{message.role === 'user' ? 'You' : 'SimiClaw'}</span>
                  <span className={styles.messageTime}>{formatDateTime(message.createdAt)}</span>
                </div>
                <div className={message.role === 'user' ? styles.userBubble : styles.assistantBubble}>
                  <div className={styles.messageContent}>{message.content || ' '}</div>
                  {message.streaming ? <span className={styles.streamingCursor} aria-hidden="true" /> : null}
                </div>
              </article>
            ))}
          </div>

          <div className={styles.composerDock}>
            <div className={styles.composerHeader}>
              <div>
                <div className={styles.composerTitle}>消息输入</div>
                <div className={styles.composerSubline}>
                  conversation_id：<span>{activeSummary.conversation}</span>
                </div>
              </div>
              <div className={styles.composerSubline}>{sending ? '流式处理中' : 'Enter 发送，Shift+Enter 换行'}</div>
            </div>
            <textarea
              className={styles.composerInput}
              value={composerText}
              onChange={(event) => setComposerText(event.target.value)}
              placeholder="输入消息，开始一次新的 agent run…"
              rows={4}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !event.shiftKey) {
                  event.preventDefault();
                  void handleSend();
                }
              }}
            />
            <div className={styles.composerFooter}>
              <div className={styles.inlineStatus}>
                {composeError ? <span className={styles.statusDanger}>{composeError}</span> : null}
                {!composeError ? <span>{runState.statusLabel}</span> : null}
              </div>
              <div className={styles.composerActions}>
                <button className={styles.ghostButton} type="button" onClick={() => setNewSessionOpen(true)} disabled={sending}>
                  切到新会话
                </button>
                <button className={styles.primaryButton} type="button" onClick={() => void handleSend()} disabled={sending || !composerText.trim()}>
                  {sending ? '发送中…' : '发送消息'}
                </button>
              </div>
            </div>
          </div>
        </section>
      </main>

      <aside className={debugClassName(debugOpen)}>
        <div className={styles.debugHeader}>
          <div>
            <div className={styles.panelTitle}>运行流</div>
            <div className={styles.panelSubtitle}>status / reasoning / tools / terminal</div>
          </div>
          <button className={styles.chromeButton} type="button" onClick={() => setDebugOpen((current) => !current)}>
            收起
          </button>
        </div>
        <div className={styles.debugStatsRow}>
          <span>{formatCount(runState.debugEntries.length)} 个流事件</span>
          <span>{runState.sessionKey ? `session=${runState.sessionKey}` : 'draft session'}</span>
        </div>
        <div className={styles.debugViewport} ref={debugViewportRef}>
          {runState.debugEntries.length === 0 ? (
            <InlineNotice title="等待流事件" body="发送消息后，这里会依次显示 accepted、status、reasoning、tool 和 terminal。" />
          ) : null}
          {runState.debugEntries.map((entry) => (
            <DebugCard key={entry.id} entry={entry} />
          ))}
        </div>
      </aside>

      {sidebarOpen ? <div className={styles.overlay} onClick={() => setSidebarOpen(false)} aria-hidden="true" /> : null}
      {debugOpen ? <div className={styles.debugOverlay} onClick={() => setDebugOpen(false)} aria-hidden="true" /> : null}
      {newSessionOpen ? (
        <div className={styles.modalBackdrop} onClick={() => setNewSessionOpen(false)}>
          <div className={styles.modalCard} onClick={(event) => event.stopPropagation()} role="dialog" aria-modal="true">
            <div className={styles.modalHeader}>
              <div>
                <div className={styles.panelTitle}>新建会话</div>
                <div className={styles.panelSubtitle}>为空时自动生成 `web-&lt;UTC时间&gt;` 风格的 conversation_id。</div>
              </div>
              <button className={styles.chromeButton} type="button" onClick={() => setNewSessionOpen(false)}>
                关闭
              </button>
            </div>
            <label className={styles.dialogField}>
              <span>conversation_id</span>
              <input
                value={newConversationInput}
                onChange={(event) => setNewConversationInput(event.target.value)}
                placeholder={generateDefaultConversationID()}
                autoFocus
              />
            </label>
            <div className={styles.dialogActions}>
              <button className={styles.ghostButton} type="button" onClick={() => setNewSessionOpen(false)}>
                取消
              </button>
              <button className={styles.primaryButton} type="button" onClick={handleCreateSession}>
                使用该会话
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function SidebarNotice({
  title,
  body,
  tone = 'neutral',
}: {
  title: string;
  body: string;
  tone?: 'neutral' | 'danger';
}) {
  return (
    <div className={tone === 'danger' ? styles.sidebarNoticeDanger : styles.sidebarNotice}>
      <div className={styles.noticeTitle}>{title}</div>
      <div className={styles.noticeBody}>{body}</div>
    </div>
  );
}

function InlineNotice({
  title,
  body,
  tone = 'neutral',
}: {
  title: string;
  body: string;
  tone?: 'neutral' | 'danger';
}) {
  return (
    <div className={tone === 'danger' ? styles.inlineNoticeDanger : styles.inlineNotice}>
      <div className={styles.noticeTitle}>{title}</div>
      <div className={styles.noticeBody}>{body}</div>
    </div>
  );
}

function DebugCard({ entry }: { entry: DebugEntry }) {
  return (
    <article className={debugCardClassName(entry)}>
      <div className={styles.debugCardHeader}>
        <span className={styles.debugCardTitle}>{entry.title}</span>
        <span className={styles.debugCardTime}>{formatDateTime(entry.at)}</span>
      </div>
      {entry.body ? <div className={styles.debugCardBody}>{entry.body}</div> : null}
      {entry.payload ? <pre className={styles.debugCardPayload}>{entry.payload}</pre> : null}
    </article>
  );
}

function buildIngestRequest(
  conversationID: string,
  seq: number,
  text: string,
  session: SessionRecord | null,
): IngestRequest {
  const channelType = session?.channel_type?.trim() || 'dm';
  const participantID = channelType === 'dm' ? session?.participant_id?.trim() || 'web_user' : undefined;

  return {
    source: 'web',
    conversation: {
      conversation_id: conversationID,
      channel_type: channelType,
      ...(participantID ? { participant_id: participantID } : {}),
    },
    ...(session?.session_key ? { session_key: session.session_key } : {}),
    idempotency_key: `web:${conversationID}:${seq}`,
    timestamp: new Date().toISOString(),
    payload: {
      type: 'message',
      text,
    },
  };
}

function generateDefaultConversationID(): string {
  const now = new Date();
  const year = now.getUTCFullYear();
  const month = String(now.getUTCMonth() + 1).padStart(2, '0');
  const day = String(now.getUTCDate()).padStart(2, '0');
  const hour = String(now.getUTCHours()).padStart(2, '0');
  const minute = String(now.getUTCMinutes()).padStart(2, '0');
  const second = String(now.getUTCSeconds()).padStart(2, '0');
  return `web-${year}${month}${day}T${hour}${minute}${second}Z`;
}

function mergeSessions(current: SessionRecord[], next: SessionRecord[]): SessionRecord[] {
  const byKey = new Map(current.map((item) => [item.session_key, item]));
  for (const item of next) {
    byKey.set(item.session_key, item);
  }
  return Array.from(byKey.values()).sort((left, right) =>
    right.last_activity_at.localeCompare(left.last_activity_at),
  );
}

function upsertSession(current: SessionRecord[], item: SessionRecord): SessionRecord[] {
  return mergeSessions(current, [item]);
}

function getSessionKeyFromURL(): string {
  if (typeof window === 'undefined') {
    return '';
  }
  return new URLSearchParams(window.location.search).get('session_key')?.trim() || '';
}

function syncSessionKeyToURL(sessionKey: string) {
  if (typeof window === 'undefined') {
    return;
  }
  const url = new URL(window.location.href);
  if (sessionKey) {
    url.searchParams.set('session_key', sessionKey);
  } else {
    url.searchParams.delete('session_key');
  }
  window.history.replaceState({}, '', `${url.pathname}${url.search}${url.hash}`);
}

function toErrorMessage(error: unknown): string {
  if (error instanceof APIError) {
    return error.code ? `${error.code}: ${error.message}` : error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return '未知错误';
}

function sidebarClassName(open: boolean): string {
  return open ? `${styles.sidebar} ${styles.sidebarOpen}` : styles.sidebar;
}

function debugClassName(open: boolean): string {
  return open ? `${styles.debugPanel} ${styles.debugPanelOpen}` : styles.debugPanel;
}

function debugCardClassName(entry: DebugEntry): string {
  switch (entry.tone) {
    case 'success':
      return `${styles.debugCard} ${styles.debugCardSuccess}`;
    case 'danger':
      return `${styles.debugCard} ${styles.debugCardDanger}`;
    case 'warning':
      return `${styles.debugCard} ${styles.debugCardWarning}`;
    default:
      return styles.debugCard;
  }
}
