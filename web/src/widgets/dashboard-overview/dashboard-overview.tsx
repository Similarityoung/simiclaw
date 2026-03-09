import { Activity, Bot, RadioTower, Workflow } from 'lucide-react';

import { MetricCard } from '@/components/shared/metric-card';
import type { RunSummary, SessionRecord } from '@/lib/api-client';

interface DashboardOverviewProps {
  healthLabel: string;
  readyLabel: string;
  sessions: SessionRecord[];
  runs: RunSummary[];
  primaryModel: string;
}

export function DashboardOverview({ healthLabel, readyLabel, sessions, runs, primaryModel }: DashboardOverviewProps): JSX.Element {
  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      <MetricCard label="Gateway" value={healthLabel} hint="来自 /healthz 的在线状态。" icon={RadioTower} />
      <MetricCard label="Workers" value={readyLabel} hint="来自 /readyz 的就绪态。" icon={Activity} />
      <MetricCard label="Active Sessions" value={String(sessions.length)} hint="最近活跃会话数量。" icon={Workflow} />
      <MetricCard label="Primary Model" value={primaryModel.replace('Model: ', '')} hint={`${runs.length} 条最近 runs 可用。`} icon={Bot} />
    </div>
  );
}
