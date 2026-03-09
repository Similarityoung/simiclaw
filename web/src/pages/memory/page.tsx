import { BookOpenText } from 'lucide-react';

import { EmptyState } from '@/components/shared/empty-state';
import { SectionHeader } from '@/components/shared/section-header';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';

export function MemoryPage(): JSX.Element {
  return (
    <div className="space-y-6">
      <SectionHeader
        eyebrow="Memory Surface"
        title="Memory"
        body="本阶段只提供列表入口页，不做 Markdown 查看器与编辑器。未来如需真正读写能力，再配合后端单独建设。"
      />

      <Card>
        <CardHeader>
          <CardTitle>记忆入口</CardTitle>
        </CardHeader>
        <CardContent>
          <EmptyState
            title="Memory 列表尚未开放"
            body="当前没有用于枚举 memory 文件的现成 API，因此这里只保留稳定说明态。"
            eyebrow="Memory"
            icon={BookOpenText}
          />
        </CardContent>
      </Card>
    </div>
  );
}
