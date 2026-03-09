import { Blocks } from 'lucide-react';

import { EmptyState } from '@/components/shared/empty-state';
import { SectionHeader } from '@/components/shared/section-header';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';

export function SkillsPage(): JSX.Element {
  return (
    <div className="space-y-6">
      <SectionHeader
        eyebrow="Extensions"
        title="Skills"
        body="本阶段只提供技能 / MCP 容器页与空状态，不实现安装、启停、编辑等管理能力。"
      />

      <Card>
        <CardHeader>
          <CardTitle>技能容器</CardTitle>
        </CardHeader>
        <CardContent>
          <EmptyState
            title="暂无可展示的技能管理数据"
            body="现有后端没有提供 Skills 管理 API，因此这里仅保留明确的空状态，不暴露假交互。"
            eyebrow="Skills"
            icon={Blocks}
          />
        </CardContent>
      </Card>
    </div>
  );
}
