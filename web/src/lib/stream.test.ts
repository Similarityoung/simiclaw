import { describe, expect, it } from 'vitest';

import { readSSE } from './stream';

function streamFrom(chunks: string[]): ReadableStream<Uint8Array> {
  return new ReadableStream<Uint8Array>({
    start(controller) {
      const encoder = new TextEncoder();
      for (const chunk of chunks) {
        controller.enqueue(encoder.encode(chunk));
      }
      controller.close();
    },
  });
}

describe('readSSE', () => {
  it('忽略 comment 并按 event/data 解析', async () => {
    const events: Array<{ eventType: string; data: string }> = [];
    await readSSE(
      streamFrom([
        ': keepalive\n\n',
        'event: accepted\n',
        'data: {"type":"accepted","event_id":"evt-1"}\n\n',
      ]),
      async (event) => {
        events.push(event);
      },
    );

    expect(events).toEqual([
      {
        eventType: 'accepted',
        data: '{"type":"accepted","event_id":"evt-1"}',
      },
    ]);
  });

  it('支持多行 data 合并', async () => {
    const events: Array<{ eventType: string; data: string }> = [];
    await readSSE(
      streamFrom([
        'event: reasoning_delta\n',
        'data: first line\n',
        'data: second line\n\n',
      ]),
      async (event) => {
        events.push(event);
      },
    );

    expect(events).toEqual([
      {
        eventType: 'reasoning_delta',
        data: 'first line\nsecond line',
      },
    ]);
  });
});
