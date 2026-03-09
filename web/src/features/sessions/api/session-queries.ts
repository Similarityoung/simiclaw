import type { QueryKey } from '@tanstack/react-query';

export const sessionKeys = {
  all: ['sessions'] as const,
  list: (limit = 20) => [...sessionKeys.all, 'list', limit] as const,
  detail: (sessionKey: string) => [...sessionKeys.all, 'detail', sessionKey] as const,
  history: (sessionKey: string, cursor?: string, visibleOnly = true) =>
    [...sessionKeys.all, 'history', sessionKey, cursor ?? 'first', visibleOnly] as const satisfies QueryKey,
};
