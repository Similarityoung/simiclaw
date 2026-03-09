import type { LucideIcon } from 'lucide-react';

import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { cn } from '@/lib/utils';

interface MetricCardProps {
  label: string;
  value: string;
  hint?: string;
  icon: LucideIcon;
  className?: string;
}

export function MetricCard({ label, value, hint, icon: Icon, className }: MetricCardProps): JSX.Element {
  return (
    <Card className={cn('min-h-[156px]', className)}>
      <CardHeader className="pb-0">
        <div className="flex items-center justify-between text-xs uppercase tracking-[0.28em] text-muted-foreground">
          <span>{label}</span>
          <Icon className="h-4 w-4" />
        </div>
      </CardHeader>
      <CardContent className="pt-4">
        <div className="text-3xl font-semibold tracking-tight">{value}</div>
        {hint ? <p className="mt-3 text-sm leading-6 text-muted-foreground">{hint}</p> : null}
      </CardContent>
    </Card>
  );
}
