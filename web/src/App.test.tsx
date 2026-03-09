import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import App from './App';
import type { RuntimeClient, SessionRecord } from './types';

function buildSession(overrides: Partial<SessionRecord> = {}): SessionRecord {
  return {
    session_key: 'session-1',
    active_session_id: 'sid-1',
    conversation_id: 'demo-conversation',
    channel_type: 'dm',
    participant_id: 'web_user',
    message_count: 2,
    prompt_tokens_total: 10,
    completion_tokens_total: 12,
    total_tokens_total: 22,
    last_model: 'fake/default',
    last_run_id: 'run-1',
    last_activity_at: '2026-03-09T12:00:00Z',
    created_at: '2026-03-09T12:00:00Z',
    updated_at: '2026-03-09T12:00:00Z',
    ...overrides,
  };
}

function buildClient(overrides: Partial<RuntimeClient> = {}): RuntimeClient {
  return {
    listSessions: vi.fn().mockResolvedValue({ items: [], next_cursor: '' }),
    getSession: vi.fn().mockResolvedValue(buildSession()),
    getSessionHistory: vi.fn().mockResolvedValue({ items: [], next_cursor: '' }),
    getEvent: vi.fn(),
    sendChat: vi.fn(),
    ...overrides,
  };
}

describe('App', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/');
  });

  it('加载会话并以 visibleOnly=true 请求历史', async () => {
    const session = buildSession();
    const client = buildClient({
      listSessions: vi.fn().mockResolvedValue({ items: [session], next_cursor: '' }),
      getSessionHistory: vi.fn().mockResolvedValue({
        items: [
          {
            message_id: 'm-1',
            session_key: session.session_key,
            session_id: session.active_session_id,
            run_id: 'run-1',
            role: 'assistant',
            content: '历史回复',
            visible: true,
            created_at: '2026-03-09T12:00:00Z',
          },
        ],
        next_cursor: '',
      }),
    });

    render(<App client={client} />);

    expect(await screen.findAllByText('demo-conversation')).not.toHaveLength(0);
    expect(await screen.findByText('历史回复')).toBeInTheDocument();
    await waitFor(() => {
      expect(client.getSessionHistory).toHaveBeenCalledWith({
        sessionKey: 'session-1',
        limit: 40,
        visibleOnly: true,
      });
    });
  });

  it('新会话首条消息发送后会回填 session_key 并展示调试流', async () => {
    const sendChat = vi.fn(async (request, options) => {
      const now = '2026-03-09T12:10:00Z';
      await options?.onEvent?.({
        type: 'accepted',
        event_id: 'evt-new',
        sequence: 1,
        at: now,
        stream_protocol_version: '2026-03-07.sse.v1',
        ingest_response: {
          event_id: 'evt-new',
          session_key: 'session-new',
          active_session_id: 'sid-new',
          received_at: now,
          payload_hash: 'hash-new',
        },
      });
      await options?.onEvent?.({
        type: 'reasoning_delta',
        event_id: 'evt-new',
        sequence: 2,
        at: now,
        delta: '先整理上下文。',
      });
      await options?.onEvent?.({
        type: 'text_delta',
        event_id: 'evt-new',
        sequence: 3,
        at: now,
        delta: '你好',
      });
      const record = {
        event_id: 'evt-new',
        status: 'processed' as const,
        session_key: 'session-new',
        session_id: 'sid-new',
        assistant_reply: '你好，世界',
        received_at: now,
        created_at: now,
        updated_at: now,
        payload_hash: 'hash-new',
      };
      await options?.onEvent?.({
        type: 'done',
        event_id: 'evt-new',
        sequence: 4,
        at: now,
        event_record: record,
      });
      return record;
    });

    const client = buildClient({
      sendChat,
      listSessions: vi
        .fn()
        .mockResolvedValueOnce({ items: [], next_cursor: '' })
        .mockResolvedValueOnce({ items: [buildSession({ session_key: 'session-new', active_session_id: 'sid-new', conversation_id: 'web-test' })], next_cursor: '' }),
    });

    render(<App client={client} />);

    const input = await screen.findByPlaceholderText('输入消息，开始一次新的 agent run…');
    await userEvent.type(input, 'hello world');
    await userEvent.click(screen.getByRole('button', { name: '发送消息' }));

    expect(await screen.findByText('hello world')).toBeInTheDocument();
    expect(await screen.findAllByText('你好，世界')).not.toHaveLength(0);
    expect(await screen.findByText('Reasoning')).toBeInTheDocument();
    expect(await screen.findByText('先整理上下文。')).toBeInTheDocument();
    await waitFor(() => {
      expect(window.location.search).toContain('session_key=session-new');
    });
  });
});
