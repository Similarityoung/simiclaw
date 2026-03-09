import type { MessageRecord, SessionRecord } from '@/lib/api-client';

export interface SessionListItem {
  key: string;
  conversation: string;
  activityAt: string;
  model: string;
  messageCount: number;
  channelLabel: string;
}

export interface ChatMessageItem {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  createdAt: string;
  streaming?: boolean;
}

export function toSessionListItem(session: SessionRecord): SessionListItem {
  return {
    key: session.session_key,
    conversation: session.conversation_id || 'untitled-session',
    activityAt: session.last_activity_at,
    model: session.last_model || 'Model · -',
    messageCount: session.message_count,
    channelLabel: session.channel_type || 'unknown',
  };
}

export function toChatMessageItem(message: MessageRecord): ChatMessageItem | null {
  if (!message.visible || (message.role !== 'user' && message.role !== 'assistant')) {
    return null;
  }

  return {
    id: message.message_id,
    role: message.role,
    content: message.content,
    createdAt: message.created_at,
  };
}
