import { useQuery } from '@tanstack/react-query';

import { runtimeKeys } from '@/features/runtime/api/runtime-queries';
import { runtimeClient } from '@/lib/api-client';

export function useRuntimeHealth() {
  const healthQuery = useQuery({
    queryKey: runtimeKeys.health(),
    queryFn: () => runtimeClient.getHealth(),
    refetchInterval: 10000,
  });

  const readyQuery = useQuery({
    queryKey: runtimeKeys.ready(),
    queryFn: () => runtimeClient.getReady(),
    refetchInterval: 10000,
  });

  return { healthQuery, readyQuery };
}
