import type { RunSummary, SessionRecord } from '@/lib/api-client';

export interface HeaderSummary {
  onlineLabel: string;
  readyLabel: string;
  primaryModel: string;
  updatedAt: string;
}

export function deriveHeaderSummary(
  health: Record<string, unknown> | undefined,
  ready: Record<string, unknown> | undefined,
  sessions: SessionRecord[] | undefined,
  runs: RunSummary[] | undefined,
): HeaderSummary {
  const healthStatus = typeof health?.status === 'string' ? health.status : 'unknown';
  const readyStatus = typeof ready?.status === 'string' ? ready.status : 'unknown';
  const mostRelevantRun = runs?.[0];
  const primaryModel = mostRelevantRun
    ? sessions?.find((item) => item.last_run_id === mostRelevantRun.run_id)?.last_model
    : undefined;

  return {
    onlineLabel: healthStatus === 'ok' ? 'Online' : healthStatus === 'unknown' ? 'Unknown' : 'Offline',
    readyLabel: readyStatus === 'ok' ? 'Ready' : readyStatus === 'unknown' ? 'Unknown' : 'Degraded',
    primaryModel: primaryModel || sessions?.[0]?.last_model || 'Model: -',
    updatedAt: new Date().toISOString(),
  };
}
