import { beforeEach, describe, expect, it, vi } from 'vitest';

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
  beforeEach(() => {
    window.history.replaceState({}, '', '/');
  });

  it('listSessions 会保留同源路径前缀', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ items: [], next_cursor: '' }));
    const client = createRuntimeClient(fetcher, { baseURL: '/simiclaw' });

    await client.listSessions({ limit: 10 });

    expect(fetcher).toHaveBeenCalledWith('/simiclaw/v1/sessions?limit=10');
  });

  it('拒绝跨域的绝对 VITE_API_BASE_URL', async () => {
    const fetcher = vi.fn<typeof fetch>();
    const client = createRuntimeClient(fetcher, { baseURL: 'https://example.com/simiclaw' });

    await expect(client.listSessions({ limit: 10 })).rejects.toThrow(
      'cross-origin VITE_API_BASE_URL unsupported; use same-origin path prefix or Vite dev proxy',
    );
    expect(fetcher).not.toHaveBeenCalled();
  });
});
