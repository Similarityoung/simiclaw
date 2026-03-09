import { useQuery } from '@tanstack/react-query';

import { sessionKeys } from '@/features/sessions/api/session-queries';
import { runtimeClient } from '@/lib/api-client';

export function useSessionHistory(sessionKey?: string, cursor?: string, visibleOnly = true) {
  return useQuery({
    queryKey: sessionKey ? sessionKeys.history(sessionKey, cursor, visibleOnly) : ['sessions', 'history', 'disabled'],
    queryFn: () => runtimeClient.getSessionHistory({ sessionKey: sessionKey ?? '', cursor, limit: 40, visibleOnly }),
    enabled: Boolean(sessionKey),
  });
}
