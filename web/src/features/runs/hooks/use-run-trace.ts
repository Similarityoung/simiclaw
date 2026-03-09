import { useQuery } from '@tanstack/react-query';

import { runKeys } from '@/features/runs/api/run-queries';
import { runtimeClient } from '@/lib/api-client';

export function useRunTrace(runID?: string) {
  return useQuery({
    queryKey: runKeys.trace(runID),
    queryFn: () => runtimeClient.getRunTrace(runID ?? ''),
    enabled: Boolean(runID),
  });
}
