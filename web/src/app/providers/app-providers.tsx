import type { ReactNode } from 'react';

import { AppErrorBoundary } from '@/app/error/app-error-boundary';
import { QueryProvider } from '@/app/providers/query-provider';
import { ThemeProvider } from '@/app/providers/theme-provider';

interface AppProvidersProps {
  children: ReactNode;
}

export function AppProviders({ children }: AppProvidersProps): JSX.Element {
  return (
    <AppErrorBoundary>
      <QueryProvider>
        <ThemeProvider>{children}</ThemeProvider>
      </QueryProvider>
    </AppErrorBoundary>
  );
}
