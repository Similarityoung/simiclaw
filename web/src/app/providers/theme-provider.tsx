import type { ReactNode } from 'react';
import { useEffect } from 'react';

import { useShellStore } from '@/app/store/shell-store';

interface ThemeProviderProps {
  children: ReactNode;
}

export function ThemeProvider({ children }: ThemeProviderProps): JSX.Element {
  const themeMode = useShellStore((state) => state.themeMode);

  useEffect(() => {
    document.documentElement.classList.toggle('dark', themeMode === 'dark');
  }, [themeMode]);

  return <>{children}</>;
}
