import { create } from 'zustand';

export type ThemeMode = 'light' | 'dark';

interface HeaderSummaryCache {
  onlineLabel: string;
  readyLabel: string;
  primaryModel: string;
  updatedAt: string;
}

interface ShellStore {
  sidebarCollapsed: boolean;
  commandPaletteOpen: boolean;
  themeMode: ThemeMode;
  headerSummaryCache?: HeaderSummaryCache;
  setSidebarCollapsed: (collapsed: boolean) => void;
  toggleSidebarCollapsed: () => void;
  setCommandPaletteOpen: (open: boolean) => void;
  setThemeMode: (mode: ThemeMode) => void;
  setHeaderSummaryCache: (summary: HeaderSummaryCache) => void;
}

export const useShellStore = create<ShellStore>((set) => ({
  sidebarCollapsed: false,
  commandPaletteOpen: false,
  themeMode: 'dark',
  headerSummaryCache: undefined,
  setSidebarCollapsed: (collapsed) => set({ sidebarCollapsed: collapsed }),
  toggleSidebarCollapsed: () => set((state) => ({ sidebarCollapsed: !state.sidebarCollapsed })),
  setCommandPaletteOpen: (open) => set({ commandPaletteOpen: open }),
  setThemeMode: (mode) => set({ themeMode: mode }),
  setHeaderSummaryCache: (summary) => set({ headerSummaryCache: summary }),
}));
