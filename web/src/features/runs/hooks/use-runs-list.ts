import { useQuery } from '@tanstack/react-query';

import { runKeys } from '@/features/runs/api/run-queries';
import { runtimeClient } from '@/lib/api-client';

export function useRunsList(sessionKey?: string, limit = 12) {
  return useQuery({
    queryKey: runKeys.list(sessionKey, limit),
    queryFn: () => runtimeClient.listRuns({ sessionKey, limit }),
    refetchInterval: 12000,
  });
}
