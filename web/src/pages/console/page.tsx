import { useEffect, useMemo, useState } from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import { ChevronRight, MessagesSquare, PanelLeftClose, PanelLeftOpen, SendHorizontal, TerminalSquare } from 'lucide-react';

import { EmptyState } from '@/components/shared/empty-state';
import { ErrorState } from '@/components/shared/error-state';
import { LoadingState } from '@/components/shared/loading-state';
import { SectionHeader } from '@/components/shared/section-header';
import { motionTokens } from '@/app/motion/tokens';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Textarea } from '@/components/ui/textarea';
import { useConsoleChat } from '@/features/chat/hooks/use-console-chat';
import { useRunTrace } from '@/features/runs/hooks/use-run-trace';
import { useRunsList } from '@/features/runs/hooks/use-runs-list';
import { useSessionHistory } from '@/features/sessions/hooks/use-session-history';
import { useSessionList } from '@/features/sessions/hooks/use-session-list';
import { toChatMessageItem, toSessionListItem } from '@/features/sessions/model/session-model';
import { ConsoleRuntime } from '@/widgets/console-runtime/console-runtime';

export function ConsolePage(): JSX.Element {
  const sessionsQuery = useSessionList(24);
  const [searchText, setSearchText] = useState('');
  const [activeSessionKey, setActiveSessionKey] = useState<string | undefined>();
  const [selectedRunID, setSelectedRunID] = useState<string | undefined>();
  const [workspaceTab, setWorkspaceTab] = useState<'conversation' | 'runtime'>('conversation');
  const [sessionRailCollapsed, setSessionRailCollapsed] = useState(false);

  const sessions = sessionsQuery.data?.items ?? [];
  const activeSession = sessions.find((item) => item.session_key === activeSessionKey) ?? sessions[0];

  useEffect(() => {
    if (!activeSessionKey && sessions[0]?.session_key) {
      setActiveSessionKey(sessions[0].session_key);
    }
  }, [activeSessionKey, sessions]);

  const historyQuery = useSessionHistory(activeSession?.session_key);
  const runsQuery = useRunsList(activeSession?.session_key, 8);
  const runTraceQuery = useRunTrace(selectedRunID);
  const chat = useConsoleChat(activeSession);

  const filteredSessions = useMemo(() => {
    const keyword = searchText.trim().toLowerCase();
    if (!keyword) {
      return sessions;
    }
    return sessions.filter((item) => {
      const payload = [item.session_key, item.conversation_id, item.last_model].filter(Boolean).join(' ').toLowerCase();
      return payload.includes(keyword);
    });
  }, [searchText, sessions]);

  const messages = useMemo(
    () => [
      ...(historyQuery.data?.items.map(toChatMessageItem).filter((item): item is NonNullable<typeof item> => item !== null) ?? []),
      ...chat.state.messages,
    ],
    [chat.state.messages, historyQuery.data?.items],
  );

  if (sessionsQuery.isLoading) {
    return <LoadingState title="初始化 Console" body="正在读取会话列表与当前工作区摘要。" />;
  }

  if (sessionsQuery.error) {
    return <ErrorState body="会话列表加载失败，Console 无法建立主操作区。" />;
  }

  return (
    <div className="space-y-5">
      <SectionHeader
        eyebrow="Primary Workspace"
        title="Console"
        body="只保留必要结构，把注意力集中到会话、对话与按需查看的 runtime。"
        action={
          <Button variant="outline" onClick={() => setSessionRailCollapsed((current) => !current)}>
            {sessionRailCollapsed ? <PanelLeftOpen className="mr-2 h-4 w-4" /> : <PanelLeftClose className="mr-2 h-4 w-4" />}
            {sessionRailCollapsed ? '展开会话列表' : '折叠会话列表'}
          </Button>
        }
      />

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_auto]">
        <div className="surface-card min-w-0 overflow-hidden bg-card p-0 backdrop-blur-none">
          <div className="flex flex-col gap-4 border-b border-border/70 px-6 py-5">
            <div className="flex items-center justify-between gap-3">
              <div className="min-w-0">
                <div className="text-[11px] uppercase tracking-[0.3em] text-muted-foreground">Workspace</div>
                <h1 className="mt-2 truncate text-2xl font-semibold tracking-tight">{activeSession?.conversation_id || chat.conversationID}</h1>
              </div>
              <div className="rounded-full border border-border px-3 py-1 text-xs text-muted-foreground">{chat.state.statusLabel}</div>
            </div>

            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => setWorkspaceTab('conversation')}
                className={`inline-flex items-center gap-2 rounded-full border px-4 py-2 text-sm transition-colors ${
                  workspaceTab === 'conversation' ? 'border-primary bg-primary text-primary-foreground' : 'border-border bg-background text-muted-foreground hover:bg-accent'
                }`}
              >
                <MessagesSquare className="h-4 w-4" />
                Conversation
              </button>
              <button
                type="button"
                onClick={() => setWorkspaceTab('runtime')}
                className={`inline-flex items-center gap-2 rounded-full border px-4 py-2 text-sm transition-colors ${
                  workspaceTab === 'runtime' ? 'border-primary bg-primary text-primary-foreground' : 'border-border bg-background text-muted-foreground hover:bg-accent'
                }`}
              >
                <TerminalSquare className="h-4 w-4" />
                Runtime
              </button>
            </div>
          </div>

          <div className="grid min-h-[560px] grid-rows-[minmax(0,1fr)_auto]">
            <div className="min-h-0 px-6 py-5">
              {workspaceTab === 'conversation' ? (
                <>
                  {historyQuery.isLoading ? (
                    <LoadingState title="同步会话历史" body="仅为当前选中的 session 读取可见历史。" className="border-0 p-0 shadow-none" />
                  ) : null}
                  {historyQuery.error ? <ErrorState body="会话历史读取失败。" className="border-0 p-0 shadow-none" /> : null}

                  <ScrollArea className="h-full pr-3">
                        <div className="space-y-4">
                          {messages.length ? (
                            messages.map((message) => (
                              <div key={message.id} className={`flex w-full ${message.role === 'user' ? 'justify-end' : 'justify-start'}`}>
                                <article
                                  className={`max-w-[85%] rounded-2xl border p-4 ${
                                    message.role === 'user' ? 'border-primary/25 bg-card' : 'border-border bg-card'
                                  }`}
                                >
                                  <div className="mb-2 flex flex-wrap items-center gap-2 text-[11px] uppercase tracking-[0.24em] text-muted-foreground">
                                    <span>{message.role === 'user' ? 'User' : 'SimiClaw'}</span>
                                    <ChevronRight className="h-3 w-3" />
                                    <span>{new Date(message.createdAt).toLocaleString('zh-CN')}</span>
                                  </div>
                                  <div className="whitespace-pre-wrap break-words text-sm leading-7">{message.content || ' '}</div>
                                </article>
                              </div>
                            ))
                          ) : (
                        <EmptyState
                          title="当前会话还没有消息"
                          body="发送第一条消息后，对话会直接铺满这里，不再被多余说明占用。"
                          eyebrow="Chat"
                          icon={MessagesSquare}
                          className="border-dashed"
                        />
                      )}
                    </div>
                  </ScrollArea>
                </>
              ) : (
                <ConsoleRuntime
                  compact
                  runs={runsQuery.data?.items ?? []}
                  selectedRunID={selectedRunID}
                  trace={runTraceQuery.data}
                  loadingTrace={runTraceQuery.isLoading}
                  onSelectRun={setSelectedRunID}
                />
              )}
            </div>

            {workspaceTab === 'conversation' ? (
              <div className="border-t border-border/70 px-6 py-4">
                <div className="mb-3 flex items-center justify-between gap-3">
                  <div className="text-sm text-muted-foreground">{chat.state.errorText || 'Enter 发送，Shift+Enter 换行'}</div>
                  <Button variant="ghost" onClick={chat.startDraftSession}>新会话</Button>
                </div>

                <div className="rounded-2xl border border-border bg-background p-3">
                  <Textarea
                    className="min-h-[108px] resize-none border-0 bg-transparent px-0 py-0 shadow-none focus-visible:ring-0"
                    value={chat.composerText}
                    onChange={(event) => chat.setComposerText(event.target.value)}
                    placeholder="输入消息，开始一次新的 agent run…"
                    onKeyDown={(event) => {
                      if (event.key === 'Enter' && !event.shiftKey) {
                        event.preventDefault();
                        void chat.send();
                      }
                    }}
                  />
                  <div className="mt-3 flex items-center justify-end gap-3">
                    <Button onClick={() => void chat.send()} disabled={chat.sending || !chat.composerText.trim()}>
                      <SendHorizontal className="mr-2 h-4 w-4" />
                      {chat.sending ? '发送中…' : '发送消息'}
                    </Button>
                  </div>
                </div>
              </div>
            ) : null}
          </div>
        </div>

        <motion.div animate={{ width: sessionRailCollapsed ? 76 : 248 }} transition={motionTokens.transition} className="min-w-0 overflow-hidden">
          <Card className="h-full overflow-hidden bg-card backdrop-blur-none">
            <CardHeader className={sessionRailCollapsed ? 'px-3' : undefined}>
              <CardTitle className={sessionRailCollapsed ? 'text-center text-sm' : undefined}>会话</CardTitle>
            </CardHeader>
            <CardContent className={`space-y-3 ${sessionRailCollapsed ? 'px-2' : ''}`}>
              <AnimatePresence initial={false} mode="wait">
                {!sessionRailCollapsed ? (
                  <motion.div
                    key="session-search"
                    initial={{ opacity: 0, y: -6 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, y: -6 }}
                    transition={motionTokens.transition}
                  >
                    <Input value={searchText} onChange={(event) => setSearchText(event.target.value)} placeholder="搜索 conversation / session / model" />
                  </motion.div>
                ) : null}
              </AnimatePresence>

              <ScrollArea className="h-[560px]">
                <div className={`space-y-2 ${sessionRailCollapsed ? '' : 'pr-3'}`}>
                  {filteredSessions.length ? (
                    filteredSessions.map((session) => {
                      const view = toSessionListItem(session);
                      const isActive = activeSession?.session_key === view.key;

                      return (
                        <button
                          key={view.key}
                          type="button"
                          onClick={() => setActiveSessionKey(view.key)}
                          title={view.conversation}
                          className={`w-full overflow-hidden rounded-xl border text-left transition-colors duration-150 ease-shell ${
                            sessionRailCollapsed ? 'px-2 py-3 text-center' : 'px-3 py-3'
                          } ${
                            isActive
                              ? 'border-foreground/20 bg-background text-foreground shadow-[inset_0_0_0_1px_rgba(255,255,255,0.04)]'
                              : 'border-border bg-background text-muted-foreground hover:text-foreground hover:bg-accent'
                          }`}
                        >
                          {sessionRailCollapsed ? (
                            <div className={`text-xs uppercase tracking-[0.2em] ${isActive ? 'font-semibold text-foreground' : 'font-medium'}`}>
                              {view.conversation.slice(0, 2)}
                            </div>
                          ) : (
                            <div className="min-w-0">
                              <div className={`truncate text-sm ${isActive ? 'font-semibold text-foreground' : 'font-medium text-foreground'}`}>
                                {view.conversation}
                              </div>
                              <div className={`mt-1 truncate text-[11px] ${isActive ? 'text-muted-foreground' : 'opacity-80'}`}>{view.model}</div>
                            </div>
                          )}
                        </button>
                      );
                    })
                  ) : (
                    <EmptyState
                      title="暂无匹配会话"
                      body={sessionRailCollapsed ? '无结果' : '这是基于真实 session 数据过滤后的结果。'}
                      eyebrow="Sessions"
                      className={sessionRailCollapsed ? 'p-3 text-center' : undefined}
                    />
                  )}
                </div>
              </ScrollArea>
            </CardContent>
          </Card>
        </motion.div>
      </div>
    </div>
  );
}
