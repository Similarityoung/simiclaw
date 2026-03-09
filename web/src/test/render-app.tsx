import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import type { ReactNode } from 'react';

import { AppErrorBoundary } from '@/app/error/app-error-boundary';
import { ThemeProvider } from '@/app/providers/theme-provider';
import { useShellStore } from '@/app/store/shell-store';

export function renderApp(initialEntries: string[] = ['/dashboard']) {
  return renderWithProviders(<div />, initialEntries);
}

export function renderWithProviders(ui: ReactNode, initialEntries: string[] = ['/']) {
  useShellStore.setState({
    sidebarCollapsed: false,
    commandPaletteOpen: false,
    themeMode: 'dark',
    headerSummaryCache: undefined,
  });

  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  });

  return {
    ...render(
      <TestProviders queryClient={queryClient}>
        <MemoryRouter initialEntries={initialEntries}>{ui}</MemoryRouter>
      </TestProviders>,
    ),
    queryClient,
  };
}

function TestProviders({ children, queryClient }: { children: ReactNode; queryClient: QueryClient }) {
  return (
    <AppErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <ThemeProvider>{children}</ThemeProvider>
      </QueryClientProvider>
    </AppErrorBoundary>
  );
}
