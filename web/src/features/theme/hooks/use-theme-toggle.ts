import { MonitorCog, MoonStar, SunMedium } from 'lucide-react';

import { useShellStore } from '@/app/store/shell-store';

export function useThemeToggle() {
  const themeMode = useShellStore((state) => state.themeMode);
  const setThemeMode = useShellStore((state) => state.setThemeMode);

  const nextMode = themeMode === 'dark' ? 'light' : 'dark';

  return {
    themeMode,
    nextMode,
    setThemeMode,
    toggleTheme: () => setThemeMode(nextMode),
    icon: themeMode === 'dark' ? MoonStar : SunMedium,
    alternateIcon: MonitorCog,
  };
}
