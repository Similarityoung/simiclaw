import { useMemo } from 'react';

import { EmptyState } from '@/components/shared/empty-state';
import { ErrorState } from '@/components/shared/error-state';
import { LoadingState } from '@/components/shared/loading-state';
import { SectionHeader } from '@/components/shared/section-header';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useRuntimeHealth } from '@/features/runtime/hooks/use-runtime-health';
import { deriveHeaderSummary } from '@/features/runtime/model/runtime-model';
import { useRunsList } from '@/features/runs/hooks/use-runs-list';
import { useSessionList } from '@/features/sessions/hooks/use-session-list';
import { DashboardOverview } from '@/widgets/dashboard-overview/dashboard-overview';
import { toSessionListItem } from '@/features/sessions/model/session-model';

export function DashboardPage(): JSX.Element {
  const { healthQuery, readyQuery } = useRuntimeHealth();
  const sessionsQuery = useSessionList(8);
  const runsQuery = useRunsList(undefined, 8);

  const headerSummary = deriveHeaderSummary(
    healthQuery.data,
    readyQuery.data,
    sessionsQuery.data?.items,
    runsQuery.data?.items,
  );
  const sessionItems = useMemo(() => (sessionsQuery.data?.items ?? []).map(toSessionListItem), [sessionsQuery.data?.items]);

  if (healthQuery.isLoading || readyQuery.isLoading || sessionsQuery.isLoading || runsQuery.isLoading) {
    return <LoadingState title="初始化 Dashboard" body="正在聚合 healthz、readyz、runs 与 sessions。" />;
  }

  if (healthQuery.error || readyQuery.error || sessionsQuery.error || runsQuery.error) {
    return <ErrorState body="Dashboard 在读取系统摘要时遇到错误，但应用壳层仍然保持稳定。" />;
  }

  return (
    <div className="space-y-6">
      <SectionHeader
        eyebrow="System Overview"
        title="Dashboard"
        body="总览页只展示现有接口能拿到的真实数据：在线状态、就绪状态、最近 runs、最近活跃 sessions，以及从近期运行派生出的主力模型。"
      />

      <DashboardOverview
        healthLabel={headerSummary.onlineLabel}
        readyLabel={headerSummary.readyLabel}
        sessions={sessionsQuery.data?.items ?? []}
        runs={runsQuery.data?.items ?? []}
        primaryModel={headerSummary.primaryModel}
      />

      <div className="grid gap-6 xl:grid-cols-[1.1fr_0.9fr]">
        <Card>
          <CardHeader>
            <CardTitle>最近 runs</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {runsQuery.data?.items.length ? (
              runsQuery.data.items.map((run) => (
                <div key={run.run_id} className="rounded-2xl border border-border bg-background/70 p-4">
                  <div className="flex items-center justify-between gap-3 text-sm">
                    <span className="font-medium">{run.status}</span>
                    <span className="text-muted-foreground">{new Date(run.started_at).toLocaleString('zh-CN')}</span>
                  </div>
                  <div className="mt-2 font-mono text-xs text-muted-foreground">{run.run_id}</div>
                </div>
              ))
            ) : (
              <EmptyState title="暂无运行记录" body="当前数据库为空时，这里会明确显示空状态。" eyebrow="Runs" />
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>最近活跃 sessions</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {sessionItems.length ? (
              sessionItems.map((session) => (
                <div key={session.key} className="rounded-2xl border border-border bg-background/70 p-4">
                  <div className="flex items-center justify-between gap-3 text-sm">
                    <span className="font-medium">{session.conversation}</span>
                    <span className="text-muted-foreground">{session.channelLabel}</span>
                  </div>
                  <div className="mt-2 text-xs text-muted-foreground">{session.model} · {session.messageCount} 条消息</div>
                </div>
              ))
            ) : (
              <EmptyState title="暂无会话" body="空库时固定显示真实空态，不额外伪造管理入口。" eyebrow="Sessions" />
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
