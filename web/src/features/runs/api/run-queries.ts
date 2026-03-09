export const runKeys = {
  all: ['runs'] as const,
  list: (sessionKey?: string, limit = 12) => [...runKeys.all, 'list', sessionKey ?? 'all', limit] as const,
  trace: (runID?: string) => [...runKeys.all, 'trace', runID ?? 'none'] as const,
};
