export function formatRelativeTime(input?: string): string {
  if (!input) {
    return '—';
  }
  const date = new Date(input);
  if (Number.isNaN(date.getTime())) {
    return '—';
  }
  const diffMs = date.getTime() - Date.now();
  const diffMinutes = Math.round(diffMs / 60000);
  const formatter = new Intl.RelativeTimeFormat('zh-CN', { numeric: 'auto' });

  if (Math.abs(diffMinutes) < 1) {
    return '刚刚';
  }
  if (Math.abs(diffMinutes) < 60) {
    return formatter.format(diffMinutes, 'minute');
  }
  const diffHours = Math.round(diffMinutes / 60);
  if (Math.abs(diffHours) < 24) {
    return formatter.format(diffHours, 'hour');
  }
  const diffDays = Math.round(diffHours / 24);
  if (Math.abs(diffDays) < 7) {
    return formatter.format(diffDays, 'day');
  }
  return formatDateTime(input);
}

export function formatDateTime(input?: string): string {
  if (!input) {
    return '—';
  }
  const date = new Date(input);
  if (Number.isNaN(date.getTime())) {
    return '—';
  }
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date);
}

export function formatCount(value: number): string {
  return new Intl.NumberFormat('zh-CN').format(value);
}

export function formatConversationLabel(input?: string): string {
  const trimmed = input?.trim();
  if (!trimmed) {
    return '未命名会话';
  }
  return trimmed;
}
