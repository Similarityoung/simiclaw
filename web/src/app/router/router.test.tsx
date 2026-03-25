import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { appRoutes } from '@/app/router';
import { RouteShell } from '@/app/router/route-shell';
import { ConsolePage } from '@/pages/console/page';
import { DashboardPage } from '@/pages/dashboard/page';
import { ChannelsPage } from '@/pages/channels/page';
import { MemoryPage } from '@/pages/memory/page';
import { SkillsPage } from '@/pages/skills/page';
import { renderWithProviders } from '@/test/render-app';

const sessionRecord = {
  session_key: 'session-1',
  active_session_id: 'sid-1',
  conversation_id: 'demo-conversation',
  channel_type: 'dm',
  participant_id: 'alice',
  dm_scope: 'private',
  message_count: 2,
  prompt_tokens_total: 10,
  completion_tokens_total: 14,
  total_tokens_total: 24,
  last_model: 'openai/deepseek-chat',
  last_run_id: 'run-1',
  last_activity_at: '2026-03-09T12:00:00Z',
  created_at: '2026-03-09T12:00:00Z',
  updated_at: '2026-03-09T12:00:00Z',
};

const runRecord = {
  run_id: 'run-1',
  event_id: 'evt-1',
  session_key: 'session-1',
  session_id: 'sid-1',
  run_mode: 'NORMAL',
  status: 'succeeded',
  started_at: '2026-03-09T12:01:00Z',
  ended_at: '2026-03-09T12:02:00Z',
};

beforeEach(() => {
  vi.restoreAllMocks();

  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url =
      typeof input === 'string'
        ? input
        : input instanceof Request
          ? input.url
          : input instanceof URL
            ? input.href
            : String(input);

    if (url.endsWith('/healthz')) {
      return new Response(JSON.stringify({ status: 'ok' }), { status: 200, headers: { 'Content-Type': 'application/json' } });
    }
    if (url.endsWith('/readyz')) {
      return new Response(JSON.stringify({ status: 'ok' }), { status: 200, headers: { 'Content-Type': 'application/json' } });
    }
    if (url.includes('/v1/sessions') && !url.includes('/history')) {
      return new Response(JSON.stringify({ items: [sessionRecord], next_cursor: '' }), { status: 200, headers: { 'Content-Type': 'application/json' } });
    }
    if (url.includes('/history')) {
      return new Response(
        JSON.stringify({
          items: [
            {
              message_id: 'm-1',
              session_key: 'session-1',
              session_id: 'sid-1',
              run_id: 'run-1',
              role: 'assistant',
              content: '历史回复',
              visible: true,
              created_at: '2026-03-09T12:01:00Z',
            },
          ],
          next_cursor: '',
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    }
    if (url.includes('/v1/runs') && !url.includes('/trace')) {
      return new Response(JSON.stringify({ items: [runRecord], next_cursor: '' }), { status: 200, headers: { 'Content-Type': 'application/json' } });
    }
    if (url.includes('/trace')) {
      return new Response(
        JSON.stringify({
          run_id: 'run-1',
          event_id: 'evt-1',
          session_key: 'session-1',
          session_id: 'sid-1',
          run_mode: 'NORMAL',
          status: 'succeeded',
          started_at: '2026-03-09T12:01:00Z',
          finished_at: '2026-03-09T12:02:00Z',
          model: 'openai/deepseek-chat',
          output_text: 'trace output',
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    }
    if (url.endsWith('/v1/chat:stream') && init?.method === 'POST') {
      const stream = new ReadableStream<Uint8Array>({
        start(controller) {
          const chunks = [
            'event: accepted\ndata: {"type":"accepted","event_id":"evt-new","sequence":1,"at":"2026-03-09T12:03:00Z","stream_protocol_version":"2026-03-07.sse.v1","ingest_response":{"event_id":"evt-new","session_key":"session-1","active_session_id":"sid-1","received_at":"2026-03-09T12:03:00Z","payload_hash":"hash"}}\n\n',
            'event: text_delta\ndata: {"type":"text_delta","event_id":"evt-new","sequence":2,"at":"2026-03-09T12:03:01Z","delta":"你好，控制台"}\n\n',
            'event: done\ndata: {"type":"done","event_id":"evt-new","sequence":3,"at":"2026-03-09T12:03:02Z","event_record":{"event_id":"evt-new","status":"processed","session_key":"session-1","session_id":"sid-1","assistant_reply":"你好，控制台","received_at":"2026-03-09T12:03:00Z","created_at":"2026-03-09T12:03:00Z","updated_at":"2026-03-09T12:03:02Z","payload_hash":"hash"}}\n\n',
          ];
          for (const chunk of chunks) {
            controller.enqueue(new TextEncoder().encode(chunk));
          }
          controller.close();
        },
      });
      return new Response(stream, { status: 200, headers: { 'Content-Type': 'text/event-stream' } });
    }

    throw new Error(`Unhandled fetch: ${url}`);
  });
});

describe('router shell', () => {
  it('Shell header 在真实数据下稳定渲染摘要', async () => {
    renderWithProviders(<RouteShell />);

    expect(await screen.findByText('Runtime Console')).toBeInTheDocument();
    expect(await screen.findByText('Online')).toBeInTheDocument();
    expect(await screen.findByText('Ready')).toBeInTheDocument();
    expect(await screen.findByText('openai/deepseek-chat')).toBeInTheDocument();
  });

  it('五个页面组件都可独立渲染', async () => {
    const cases = [
      { ui: <DashboardPage />, heading: 'Dashboard' },
      { ui: <ConsolePage />, heading: 'Console' },
      { ui: <ChannelsPage />, heading: 'Channels' },
      { ui: <MemoryPage />, heading: 'Memory' },
      { ui: <SkillsPage />, heading: 'Skills' },
    ] as const;

    for (const testCase of cases) {
      renderWithProviders(testCase.ui);
      expect(await screen.findByRole('heading', { name: testCase.heading, level: 1 })).toBeInTheDocument();
    }
  });

  it('Console 在新布局下可发送消息并渲染右侧空状态', async () => {
    const user = userEvent.setup();
    renderWithProviders(<ConsolePage />);

    expect(await screen.findByRole('heading', { name: 'Console', level: 1 })).toBeInTheDocument();
    expect(await screen.findByRole('button', { name: 'Conversation' })).toBeInTheDocument();
    expect(await screen.findByRole('button', { name: 'Runtime' })).toBeInTheDocument();

    const input = await screen.findByPlaceholderText('输入消息，开始一次新的 agent run…');
    await user.type(input, 'hello');
    await user.click(screen.getByRole('button', { name: /发送消息/i }));

    expect(await screen.findByText('hello')).toBeInTheDocument();
    expect(await screen.findAllByText('你好，控制台')).not.toHaveLength(0);
  });

  it('Console 在 stream endpoint 不可用时仍通过 consumer fallback 完成发送', async () => {
    const originalFetch = globalThis.fetch;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const url =
        typeof input === 'string'
          ? input
          : input instanceof Request
            ? input.url
            : input instanceof URL
              ? input.href
              : String(input);

      if (url.endsWith('/v1/chat:stream') && init?.method === 'POST') {
        return new Response('', { status: 404 });
      }
      if (url.endsWith('/v1/events:ingest') && init?.method === 'POST') {
        return new Response(
          JSON.stringify({
            event_id: 'evt-fallback',
            session_key: 'session-1',
            active_session_id: 'sid-1',
            received_at: '2026-03-09T12:05:00Z',
            payload_hash: 'hash-fallback',
            status: 'accepted',
            status_url: '/v1/events/evt-fallback',
          }),
          { status: 202, headers: { 'Content-Type': 'application/json' } },
        );
      }
      if (url.includes('/v1/events/evt-fallback')) {
        return new Response(
          JSON.stringify({
            event_id: 'evt-fallback',
            status: 'processed',
            session_key: 'session-1',
            session_id: 'sid-1',
            assistant_reply: '你好，回退链路',
            received_at: '2026-03-09T12:05:00Z',
            created_at: '2026-03-09T12:05:00Z',
            updated_at: '2026-03-09T12:05:02Z',
            payload_hash: 'hash-fallback',
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        );
      }
      return originalFetch(input, init);
    });

    const user = userEvent.setup();
    renderWithProviders(<ConsolePage />);

    const input = await screen.findByPlaceholderText('输入消息，开始一次新的 agent run…');
    await user.type(input, 'fallback');
    await user.click(screen.getByRole('button', { name: /发送消息/i }));

    expect(await screen.findByText('fallback')).toBeInTheDocument();
    expect(await screen.findAllByText('你好，回退链路')).not.toHaveLength(0);
  });

  it('Channels / Memory / Skills 在空数据时显示真实空态', async () => {
    renderWithProviders(<ChannelsPage />);
    expect(await screen.findByText('暂无通道摘要')).toBeInTheDocument();

    renderWithProviders(<MemoryPage />);
    expect(await screen.findByText('Memory 列表尚未开放')).toBeInTheDocument();

    renderWithProviders(<SkillsPage />);
    expect(await screen.findByText('暂无可展示的技能管理数据')).toBeInTheDocument();
  });

  it('路由定义包含五个懒加载页面', async () => {
    const shellRoute = appRoutes.find((route) => route.path === '/' && 'children' in route && Array.isArray(route.children));
    expect(shellRoute).toBeTruthy();

    const childPaths = shellRoute?.children?.map((route) => route.path);
    expect(childPaths).toEqual(['dashboard', 'console', 'channels', 'memory', 'skills']);
    for (const route of shellRoute?.children ?? []) {
      expect(typeof route.lazy).toBe('function');
    }
  });
});
