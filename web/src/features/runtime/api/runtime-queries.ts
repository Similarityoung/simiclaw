export const runtimeKeys = {
  all: ['runtime'] as const,
  health: () => [...runtimeKeys.all, 'health'] as const,
  ready: () => [...runtimeKeys.all, 'ready'] as const,
  header: () => [...runtimeKeys.all, 'header-summary'] as const,
};
