import { RadioTower } from 'lucide-react';

import { EmptyState } from '@/components/shared/empty-state';
import { SectionHeader } from '@/components/shared/section-header';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useSessionList } from '@/features/sessions/hooks/use-session-list';

export function ChannelsPage(): JSX.Element {
  const sessionsQuery = useSessionList(40);
  const channels = Array.from(new Set((sessionsQuery.data?.items ?? []).map((item) => item.channel_type).filter(Boolean)));

  return (
    <div className="space-y-6">
      <SectionHeader
        eyebrow="Observed Integrations"
        title="Channels"
        body="这是只读观察页：只展示当前已经被会话数据观测到的通道类型与摘要，不提供伪造的管理动作。"
      />
      <Card>
        <CardHeader>
          <CardTitle>已观测通道</CardTitle>
        </CardHeader>
        <CardContent>
          {channels.length ? (
            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
              {channels.map((channel) => (
                <div key={channel} className="rounded-2xl border border-border bg-background/70 p-5">
                  <div className="flex items-center gap-2 text-sm font-medium">
                    <RadioTower className="h-4 w-4" />
                    {channel}
                  </div>
                  <p className="mt-2 text-sm leading-6 text-muted-foreground">来自最近活跃会话的真实观测结果。</p>
                </div>
              ))}
            </div>
          ) : (
            <EmptyState title="暂无通道摘要" body="没有可用管理 API 时，这一页保持空状态说明。" eyebrow="Channels" />
          )}
        </CardContent>
      </Card>
    </div>
  );
}
