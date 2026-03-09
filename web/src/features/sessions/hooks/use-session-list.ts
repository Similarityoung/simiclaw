import { useQuery } from '@tanstack/react-query';

import { runtimeClient } from '@/lib/api-client';
import { sessionKeys } from '@/features/sessions/api/session-queries';

export function useSessionList(limit = 24) {
  return useQuery({
    queryKey: sessionKeys.list(limit),
    queryFn: () => runtimeClient.listSessions({ limit }),
    refetchInterval: 15000,
  });
}
