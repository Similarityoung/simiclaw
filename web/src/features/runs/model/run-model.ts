import type { RunSummary, RunTrace } from '@/lib/api-client';

export interface RunTerminalSnapshot {
  id: string;
  title: string;
  status: string;
  startedAt: string;
  endedAt: string;
}

export function toRunTerminalSnapshot(run: RunSummary): RunTerminalSnapshot {
  return {
    id: run.run_id,
    title: `${run.run_mode} · ${run.status}`,
    status: run.status,
    startedAt: run.started_at,
    endedAt: run.ended_at,
  };
}

export function summarizeRunTrace(trace?: RunTrace): string {
  if (!trace) {
    return '选择一条 run 后，这里才会按需读取终端快照。';
  }

  if (trace.error?.message) {
    return trace.error.message;
  }

  return trace.output_text?.trim() || '当前 run 没有可展示的输出文本。';
}
