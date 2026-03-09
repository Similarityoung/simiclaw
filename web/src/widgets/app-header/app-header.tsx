import { useMemo } from 'react';
import { Clock3, MoonStar, ServerCog, SunMedium } from 'lucide-react';

import { useShellStore } from '@/app/store/shell-store';
import { useThemeToggle } from '@/features/theme/hooks/use-theme-toggle';
import { Button } from '@/components/ui/button';

interface AppHeaderProps {
  onlineLabel: string;
  readyLabel: string;
  primaryModel: string;
  updatedAt: string;
}

export function AppHeader({ onlineLabel, readyLabel, primaryModel, updatedAt }: AppHeaderProps): JSX.Element {
  const { themeMode, toggleTheme } = useThemeToggle();
  const setCommandPaletteOpen = useShellStore((state) => state.setCommandPaletteOpen);

  const updatedLabel = useMemo(() => {
    if (!updatedAt) {
      return '未同步';
    }
    return new Date(updatedAt).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
  }, [updatedAt]);

  return (
    <header className="surface-shell sticky top-6 z-header flex min-h-[var(--header-height)] items-center justify-between rounded-[2rem] px-6 py-4">
      <div>
        <div className="text-xs uppercase tracking-[0.3em] text-muted-foreground">Control Layer</div>
        <div className="mt-2 flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
          <span className="rounded-full border border-border px-3 py-1">{onlineLabel}</span>
          <span className="rounded-full border border-border px-3 py-1">{readyLabel}</span>
          <span className="rounded-full border border-border px-3 py-1">{primaryModel}</span>
        </div>
      </div>

      <div className="flex items-center gap-3">
        <div className="hidden items-center gap-2 rounded-full border border-border px-3 py-2 text-xs text-muted-foreground md:flex">
          <Clock3 className="h-3.5 w-3.5" />
          {updatedLabel}
        </div>
        <Button variant="outline" size="sm" onClick={() => setCommandPaletteOpen(true)}>
          <ServerCog className="mr-2 h-4 w-4" />
          Command
        </Button>
        <Button variant="outline" size="icon" onClick={toggleTheme} aria-label="toggle theme">
          {themeMode === 'dark' ? <SunMedium className="h-4 w-4" /> : <MoonStar className="h-4 w-4" />}
        </Button>
      </div>
    </header>
  );
}
