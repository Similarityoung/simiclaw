import { TerminalSquare } from 'lucide-react';

import { EmptyState } from '@/components/shared/empty-state';
import { LoadingState } from '@/components/shared/loading-state';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ScrollArea } from '@/components/ui/scroll-area';
import { summarizeRunTrace } from '@/features/runs/model/run-model';
import type { RunSummary, RunTrace } from '@/lib/api-client';

interface ConsoleRuntimeProps {
  runs: RunSummary[];
  selectedRunID?: string;
  trace?: RunTrace;
  loadingTrace: boolean;
  onSelectRun: (runID: string) => void;
  compact?: boolean;
}

export function ConsoleRuntime({ runs, selectedRunID, trace, loadingTrace, onSelectRun, compact = false }: ConsoleRuntimeProps): JSX.Element {
  const content = (
    <>
      {!compact ? (
        <CardHeader>
          <div className="flex items-center justify-between gap-3">
            <div>
              <div className="text-xs uppercase tracking-[0.28em] text-muted-foreground">Runtime Snapshot</div>
              <CardTitle className="mt-2">运行终端快照</CardTitle>
            </div>
            <TerminalSquare className="h-5 w-5 text-muted-foreground" />
          </div>
        </CardHeader>
      ) : (
        <div className="mb-4 flex items-center justify-between gap-3">
          <div>
            <div className="text-xs uppercase tracking-[0.28em] text-muted-foreground">Runtime Snapshot</div>
            <h2 className="mt-2 text-xl font-semibold tracking-tight">运行终端快照</h2>
          </div>
          <TerminalSquare className="h-5 w-5 text-muted-foreground" />
        </div>
      )}

      <CardContent className={`grid gap-4 ${compact ? 'px-0 pb-0 pt-0' : ''} lg:grid-cols-[180px_minmax(0,1fr)]`}>
        <div className="space-y-2">
          {runs.length === 0 ? (
            <EmptyState title="暂无 runs" body="当前库里还没有可展示的运行记录。" eyebrow="Runs" />
          ) : (
            runs.map((run) => (
              <button
                key={run.run_id}
                type="button"
                onClick={() => onSelectRun(run.run_id)}
                className={`w-full rounded-xl border px-3 py-3 text-left transition-colors ${
                  selectedRunID === run.run_id ? 'border-primary bg-primary text-primary-foreground' : 'border-border bg-background hover:bg-accent'
                }`}
              >
                <div className="text-sm font-medium">{run.status}</div>
                <div className="mt-1 truncate text-[11px] opacity-80">{run.run_id}</div>
              </button>
            ))
          )}
        </div>

        <div className="rounded-[1.5rem] border border-border bg-background p-4">
          {loadingTrace ? <LoadingState title="读取 run trace" body="仅在选中 run 时按需请求。" className="border-0 shadow-none" /> : null}
          {!loadingTrace && !selectedRunID ? (
            <EmptyState title="选择一条 run" body="这里不会做全局实时，只展示按需读取的只读快照。" eyebrow="Trace" />
          ) : null}
          {!loadingTrace && selectedRunID ? (
            <ScrollArea className="h-[340px] pr-3">
              <pre className="whitespace-pre-wrap text-sm leading-7 text-muted-foreground">{summarizeRunTrace(trace)}</pre>
            </ScrollArea>
          ) : null}
        </div>
      </CardContent>
    </>
  );

  return (
    compact ? <div className="h-full">{content}</div> : <Card className="min-h-[280px]">{content}</Card>
  );
}
