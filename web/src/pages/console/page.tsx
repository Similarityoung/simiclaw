import { useEffect, useMemo, useState } from 'react';
import { ChevronRight, MessagesSquare, SendHorizontal, ShieldEllipsis, SplitSquareVertical, Waves } from 'lucide-react';

import { EmptyState } from '@/components/shared/empty-state';
import { ErrorState } from '@/components/shared/error-state';
import { LoadingState } from '@/components/shared/loading-state';
import { SectionHeader } from '@/components/shared/section-header';
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
  const [rightPanelCollapsed, setRightPanelCollapsed] = useState(false);

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
    <div className="space-y-6">
      <SectionHeader
        eyebrow="Primary Workspace"
        title="Console"
        body="这是唯一主操作区：左侧负责会话、对话与运行快照，右侧负责通道摘要与授权等待队列。拿不到数据的部分会明确显示空状态。"
        action={
          <Button variant="outline" onClick={() => setRightPanelCollapsed((current) => !current)}>
            <SplitSquareVertical className="mr-2 h-4 w-4" />
            {rightPanelCollapsed ? '展开右侧' : '折叠右侧'}
          </Button>
        }
      />

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1.5fr)_minmax(320px,1fr)]">
        <div className="grid min-w-0 gap-6">
          <div className="grid gap-6 xl:grid-cols-[280px_minmax(0,1fr)]">
            <Card>
              <CardHeader>
                <CardTitle>会话列表</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <Input value={searchText} onChange={(event) => setSearchText(event.target.value)} placeholder="搜索 conversation / session / model" />
                <ScrollArea className="h-[420px] pr-3">
                  <div className="space-y-2">
                    {filteredSessions.length ? (
                      filteredSessions.map((session) => {
                        const view = toSessionListItem(session);
                        return (
                          <button
                            key={view.key}
                            type="button"
                            onClick={() => setActiveSessionKey(view.key)}
                            className={`w-full rounded-2xl border px-4 py-3 text-left transition-colors ${
                              activeSession?.session_key === view.key ? 'border-primary bg-primary text-primary-foreground' : 'border-border bg-background hover:bg-accent'
                            }`}
                          >
                            <div className="text-sm font-medium">{view.conversation}</div>
                            <div className="mt-1 text-xs opacity-80">{view.model}</div>
                            <div className="mt-2 text-[11px] uppercase tracking-[0.24em] opacity-70">{view.channelLabel}</div>
                          </button>
                        );
                      })
                    ) : (
                      <EmptyState title="暂无匹配会话" body="这是基于真实 session 数据过滤后的结果。" eyebrow="Sessions" />
                    )}
                  </div>
                </ScrollArea>
              </CardContent>
            </Card>

            <Card className="min-w-0">
              <CardHeader>
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <div className="text-xs uppercase tracking-[0.28em] text-muted-foreground">Conversation</div>
                    <CardTitle className="mt-2">{activeSession?.conversation_id || chat.conversationID}</CardTitle>
                  </div>
                  <div className="rounded-full border border-border px-3 py-1 text-xs text-muted-foreground">{chat.state.statusLabel}</div>
                </div>
              </CardHeader>
              <CardContent className="space-y-4">
                {historyQuery.isLoading ? <LoadingState title="同步会话历史" body="仅为当前选中的 session 读取可见历史。" className="border-0 shadow-none" /> : null}
                {historyQuery.error ? <ErrorState body="会话历史读取失败。" /> : null}
                <ScrollArea className="h-[420px] pr-3">
                  <div className="space-y-4">
                    {messages.length ? (
                      messages.map((message) => (
                        <article key={message.id} className={`rounded-2xl border p-4 ${message.role === 'user' ? 'border-primary/30 bg-primary/5' : 'border-border bg-background/70'}`}>
                          <div className="mb-2 flex items-center gap-2 text-xs uppercase tracking-[0.24em] text-muted-foreground">
                            <span>{message.role === 'user' ? 'User' : 'SimiClaw'}</span>
                            <ChevronRight className="h-3 w-3" />
                            <span>{new Date(message.createdAt).toLocaleString('zh-CN')}</span>
                          </div>
                          <div className="whitespace-pre-wrap text-sm leading-7">{message.content || ' '}</div>
                        </article>
                      ))
                    ) : (
                      <EmptyState title="当前会话还没有消息" body="发送第一条消息后，对话区会展示真实历史与流式回复。" eyebrow="Chat" icon={MessagesSquare} />
                    )}
                  </div>
                </ScrollArea>

                <div className="rounded-[1.5rem] border border-border bg-background/80 p-4">
                  <div className="mb-3 flex items-center justify-between gap-3">
                    <div>
                      <div className="text-xs uppercase tracking-[0.28em] text-muted-foreground">Composer</div>
                      <div className="mt-1 text-sm text-muted-foreground">在 Console 内沿用现有 `chat:stream` 接口发送消息。</div>
                    </div>
                    <Button variant="ghost" onClick={chat.startDraftSession}>新会话</Button>
                  </div>
                  <Textarea
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
                  <div className="mt-3 flex items-center justify-between gap-3">
                    <div className="text-sm text-muted-foreground">{chat.state.errorText || 'Enter 发送，Shift+Enter 换行'}</div>
                    <Button onClick={() => void chat.send()} disabled={chat.sending || !chat.composerText.trim()}>
                      <SendHorizontal className="mr-2 h-4 w-4" />
                      {chat.sending ? '发送中…' : '发送消息'}
                    </Button>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>

          <ConsoleRuntime
            runs={runsQuery.data?.items ?? []}
            selectedRunID={selectedRunID}
            trace={runTraceQuery.data}
            loadingTrace={runTraceQuery.isLoading}
            onSelectRun={setSelectedRunID}
          />
        </div>

        {!rightPanelCollapsed ? (
          <div className="grid gap-6">
            <Card>
              <CardHeader>
                <div className="flex items-center gap-2 text-xs uppercase tracking-[0.28em] text-muted-foreground">
                  <Waves className="h-4 w-4" />
                  Active Channels
                </div>
                <CardTitle className="mt-2">活跃通道卡片</CardTitle>
              </CardHeader>
              <CardContent>
                {activeSession ? (
                  <div className="rounded-2xl border border-border bg-background/70 p-4">
                    <div className="text-sm font-medium">{activeSession.channel_type || 'unknown'}</div>
                    <div className="mt-2 text-sm text-muted-foreground">session_key：{activeSession.session_key}</div>
                    <div className="mt-1 text-sm text-muted-foreground">participant：{activeSession.participant_id || '-'}</div>
                  </div>
                ) : (
                  <EmptyState title="暂无可观测通道" body="没有管理 API 时，这里仅显示已观测到的真实通道摘要。" eyebrow="Channels" />
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <div className="flex items-center gap-2 text-xs uppercase tracking-[0.28em] text-muted-foreground">
                  <ShieldEllipsis className="h-4 w-4" />
                  Approval Queue
                </div>
                <CardTitle className="mt-2">授权等待队列</CardTitle>
              </CardHeader>
              <CardContent>
                <EmptyState
                  title="当前没有可展示的审批队列"
                  body="本阶段不伪造审批流；没有后端能力时，这里始终保持清晰的高亮空状态。"
                  eyebrow="Empty Queue"
                />
              </CardContent>
            </Card>
          </div>
        ) : null}
      </div>
    </div>
  );
}
