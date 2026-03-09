import { describe, expect, it } from 'vitest';

import { appendUserMessage, applyStreamEvent, createInitialRunState } from './chatState';
import type { EventRecord } from '../types';

describe('chatState', () => {
  it('accepted -> text_delta -> done 能收敛最终 assistant 内容', () => {
    const record: EventRecord = {
      event_id: 'evt-1',
      status: 'processed',
      session_key: 'session-1',
      session_id: 'sid-1',
      assistant_reply: '你好，世界',
      received_at: '2026-03-09T12:00:00Z',
      created_at: '2026-03-09T12:00:00Z',
      updated_at: '2026-03-09T12:00:05Z',
      payload_hash: 'hash-1',
    };

    let state = createInitialRunState();
    state = appendUserMessage(state, 'hello');
    state = applyStreamEvent(state, {
      type: 'accepted',
      event_id: 'evt-1',
      sequence: 1,
      at: '2026-03-09T12:00:00Z',
      stream_protocol_version: '2026-03-07.sse.v1',
      ingest_response: {
        event_id: 'evt-1',
        session_key: 'session-1',
        active_session_id: 'sid-1',
        received_at: '2026-03-09T12:00:00Z',
        payload_hash: 'hash-1',
      },
    });
    state = applyStreamEvent(state, {
      type: 'text_delta',
      event_id: 'evt-1',
      sequence: 2,
      at: '2026-03-09T12:00:01Z',
      delta: '你好',
    });
    state = applyStreamEvent(state, {
      type: 'done',
      event_id: 'evt-1',
      sequence: 3,
      at: '2026-03-09T12:00:05Z',
      event_record: record,
    });

    expect(state.messages).toHaveLength(2);
    expect(state.messages[1]).toMatchObject({
      role: 'assistant',
      content: '你好，世界',
      streaming: false,
    });
    expect(state.statusLabel).toBe('运行完成');
  });

  it('tool_result(error) 会进入危险态调试卡片', () => {
    const state = applyStreamEvent(createInitialRunState(), {
      type: 'tool_result',
      event_id: 'evt-2',
      sequence: 4,
      at: '2026-03-09T12:10:00Z',
      tool_call_id: 'tool-1',
      tool_name: 'memory_search',
      error: {
        code: 'tool_failed',
        message: 'boom',
      },
    });

    expect(state.debugEntries).toHaveLength(1);
    expect(state.debugEntries[0]).toMatchObject({
      kind: 'tool_result',
      tone: 'danger',
      body: 'tool_failed: boom',
    });
  });
});
