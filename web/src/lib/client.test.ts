import { describe, expect, it, vi } from 'vitest';

import { createRuntimeClient } from './client';

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: {
      'Content-Type': 'application/json',
    },
  });
}

describe('client', () => {
  it('listSessions 会保留 baseURL 路径前缀并附带 Bearer 鉴权头', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ items: [], next_cursor: '' }));
    const client = createRuntimeClient(fetcher, {
      baseURL: 'https://example.com/simiclaw',
      apiKey: 'secret-key',
    });

    await client.listSessions({ limit: 10 });

    expect(fetcher).toHaveBeenCalledWith('https://example.com/simiclaw/v1/sessions?limit=10', {
      headers: {
        Authorization: 'Bearer secret-key',
      },
    });
  });
});
