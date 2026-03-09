import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import ChatHeader from './components/ChatHeader';
import ChatMessageList from './components/ChatMessageList';
import Composer from './components/Composer';
import DebugPanel from './components/DebugPanel';
import NewSessionModal from './components/NewSessionModal';
import SessionSidebar from './components/SessionSidebar';
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
  formatDateTime,
  formatRelativeTime,
} from './lib/format';
import { cn } from './lib/ui';
import type {
  ChatMessageItem,
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
  const [viewportWidth, setViewportWidth] = useState(() =>
    typeof window === 'undefined' ? 1440 : window.innerWidth,
  );

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
  const sidebarPersistent = viewportWidth >= 960;
  const debugPersistent = viewportWidth >= 1280;

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
    if (typeof window === 'undefined') {
      return;
    }

    const handleResize = () => {
      setViewportWidth(window.innerWidth);
    };

    handleResize();
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

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
    <div className="relative min-h-screen overflow-hidden px-4 py-4 text-[var(--color-ink)] sm:px-5 sm:py-5 xl:px-6 xl:py-6">
      <div
        className="pointer-events-none fixed inset-0"
        aria-hidden="true"
        style={{
          background:
            'radial-gradient(circle at 10% 16%, rgba(124, 147, 255, 0.18), transparent 22%), radial-gradient(circle at 88% 10%, rgba(255,255,255,0.05), transparent 18%), radial-gradient(circle at 72% 86%, rgba(80,208,160,0.08), transparent 18%)',
        }}
      />
      <main className="ui-panel-strong relative mx-auto flex min-h-[calc(100vh-2rem)] w-full max-w-[1800px] flex-col overflow-hidden p-5 md:min-h-[calc(100vh-2.5rem)] md:p-6 xl:grid xl:grid-cols-[minmax(18rem,22rem)_minmax(0,1fr)_minmax(21rem,24rem)] xl:items-stretch xl:gap-6 xl:p-6">
        <SessionSidebar
          sessions={sessions}
          filteredSessions={filteredSessions}
          activeSessionKey={activeSessionKey}
          searchText={searchText}
          sessionsCursor={sessionsCursor}
          sessionsLoading={sessionsLoading}
          sessionsBusy={sessionsBusy}
          sessionsError={sessionsError}
          sending={sending}
          open={sidebarPersistent || sidebarOpen}
          persistent={sidebarPersistent}
          onOpenNewSession={() => setNewSessionOpen(true)}
          onRefresh={() => void refreshSessions()}
          onSearchChange={setSearchText}
          onOpenSession={(session) => void openSession(session)}
          onLoadMore={() => void loadMoreSessions()}
        />

        <div className="relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-[28px] border border-white/8 bg-[linear-gradient(180deg,rgba(10,13,20,0.52),rgba(6,8,13,0.24))] px-4 py-5 sm:px-5 sm:py-6 xl:h-[calc(100vh-3rem)] xl:px-7 xl:py-7">
        <ChatHeader
          conversation={activeSummary.conversation}
          status={topbarStatus}
          lastActivity={activeSummary.lastActivity}
          model={activeSummary.model}
          onToggleSidebar={() => {
            if (!sidebarPersistent) {
              setSidebarOpen((current) => !current);
            }
          }}
          onToggleDebug={() => {
            if (!debugPersistent) {
              setDebugOpen((current) => !current);
            }
          }}
        />

        <section className="flex min-h-0 flex-1 flex-col gap-5 overflow-hidden pt-6 xl:pt-7">
          <ChatMessageList
            messages={runState.messages}
            historyCursor={historyCursor}
            historyLoading={historyLoading}
            historyLoadingMore={historyLoadingMore}
            historyError={historyError}
            empty={runState.messages.length === 0}
            onLoadMore={() => void loadMoreHistory()}
            scrollRef={messagesViewportRef}
          />

          <Composer
            conversationLabel={activeSummary.conversation}
            composerText={composerText}
            sending={sending}
            composeError={composeError}
            statusLabel={runState.statusLabel}
            onChange={setComposerText}
            onSend={() => void handleSend()}
            onOpenNewSession={() => setNewSessionOpen(true)}
          />
        </section>
        </div>

        <DebugPanel
          debugEntries={runState.debugEntries.map((entry) => ({
            ...entry,
            at: formatDateTime(entry.at),
          }))}
          sessionKey={runState.sessionKey}
          open={debugPersistent || debugOpen}
          persistent={debugPersistent}
          scrollRef={debugViewportRef}
          onToggle={() => {
            if (!debugPersistent) {
              setDebugOpen((current) => !current);
            }
          }}
        />
      </main>

      {!sidebarPersistent && sidebarOpen ? (
        <div
          className="fixed inset-0 z-20 bg-[rgba(4,6,10,0.66)] backdrop-blur-lg md:hidden"
          onClick={() => setSidebarOpen(false)}
          aria-hidden="true"
        />
      ) : null}
      {!debugPersistent && debugOpen ? (
        <div
          className={cn(
            'fixed inset-0 z-10 bg-[rgba(4,6,10,0.58)] backdrop-blur-lg',
            'hidden max-xl:block',
          )}
          onClick={() => setDebugOpen(false)}
          aria-hidden="true"
        />
      ) : null}

      <NewSessionModal
        open={newSessionOpen}
        value={newConversationInput}
        placeholder={generateDefaultConversationID()}
        onClose={() => setNewSessionOpen(false)}
        onChange={setNewConversationInput}
        onConfirm={handleCreateSession}
      />
    </div>
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
