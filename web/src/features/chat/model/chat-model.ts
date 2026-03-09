import type { ChatStreamEvent, EventRecord, IngestRequest, SessionRecord } from '@/lib/api-client';

export interface DebugEntry {
  id: string;
  kind: ChatStreamEvent['type'] | 'system';
  title: string;
  body?: string;
  payload?: string;
  at: string;
}

export interface ConsoleChatState {
  messages: Array<{
    id: string;
    role: 'user' | 'assistant';
    content: string;
    createdAt: string;
    streaming?: boolean;
  }>;
  debugEntries: DebugEntry[];
  assistantMessageID?: string;
  sessionKey?: string;
  statusLabel: string;
  errorText?: string;
}

export function createInitialConsoleChatState(): ConsoleChatState {
  return {
    messages: [],
    debugEntries: [],
    statusLabel: '等待输入',
  };
}

export function buildIngestRequest(conversationID: string, sequence: number, text: string, session?: SessionRecord): IngestRequest {
  return {
    source: 'web',
    conversation: {
      conversation_id: conversationID,
      channel_type: session?.channel_type || 'dm',
      participant_id: session?.participant_id || 'web-user',
    },
    ...(session?.session_key ? { session_key: session.session_key } : {}),
    idempotency_key: `web:${conversationID}:${sequence}`,
    timestamp: new Date().toISOString(),
    payload: {
      type: 'message',
      text,
    },
  };
}

export function createConversationID(): string {
  const now = new Date();
  const parts = [
    now.getUTCFullYear(),
    String(now.getUTCMonth() + 1).padStart(2, '0'),
    String(now.getUTCDate()).padStart(2, '0'),
    'T',
    String(now.getUTCHours()).padStart(2, '0'),
    String(now.getUTCMinutes()).padStart(2, '0'),
    String(now.getUTCSeconds()).padStart(2, '0'),
    'Z',
  ];
  return `web-${parts.join('')}`;
}

export function appendUserMessage(state: ConsoleChatState, text: string): ConsoleChatState {
  return {
    ...state,
    statusLabel: '请求发送中',
    errorText: undefined,
    messages: [
      ...state.messages,
      {
        id: `user-${Date.now()}`,
        role: 'user',
        content: text,
        createdAt: new Date().toISOString(),
      },
    ],
  };
}

export function applyChatStreamEvent(state: ConsoleChatState, event: ChatStreamEvent): ConsoleChatState {
  const nextState: ConsoleChatState = {
    ...state,
    sessionKey: event.ingest_response?.session_key || event.event_record?.session_key || state.sessionKey,
  };

  switch (event.type) {
    case 'accepted':
      return {
        ...nextState,
        statusLabel: '请求已接收',
        debugEntries: [
          ...nextState.debugEntries,
          { id: `accepted-${event.event_id}`, kind: event.type, title: '请求已接收', body: event.event_id, at: event.at },
        ],
      };
    case 'status':
      return {
        ...nextState,
        statusLabel: event.message || event.status || '处理中',
        debugEntries: [
          ...nextState.debugEntries,
          {
            id: `status-${event.sequence}`,
            kind: event.type,
            title: '状态更新',
            body: event.message || event.status || '处理中',
            at: event.at,
          },
        ],
      };
    case 'reasoning_delta':
      return {
        ...nextState,
        statusLabel: '模型思考中',
        debugEntries: [
          ...nextState.debugEntries,
          { id: `reasoning-${event.sequence}`, kind: event.type, title: 'Reasoning', body: event.delta, at: event.at },
        ],
      };
    case 'text_delta': {
      const delta = event.delta ?? '';
      if (!delta) {
        return nextState;
      }
      if (!nextState.assistantMessageID) {
        const messageID = `assistant-${Date.now()}`;
        return {
          ...nextState,
          assistantMessageID: messageID,
          statusLabel: '回复生成中',
          messages: [
            ...nextState.messages,
            { id: messageID, role: 'assistant', content: delta, createdAt: event.at, streaming: true },
          ],
        };
      }
      return {
        ...nextState,
        statusLabel: '回复生成中',
        messages: nextState.messages.map((item) =>
          item.id === nextState.assistantMessageID ? { ...item, content: `${item.content}${delta}`, streaming: true } : item,
        ),
      };
    }
    case 'tool_start':
    case 'tool_result':
      return {
        ...nextState,
        statusLabel: event.type === 'tool_start' ? `调用工具 ${event.tool_name || 'unknown'}` : '工具执行完成',
        debugEntries: [
          ...nextState.debugEntries,
          {
            id: `${event.type}-${event.sequence}`,
            kind: event.type,
            title: event.type === 'tool_start' ? `工具开始 · ${event.tool_name || 'unknown'}` : `工具结果 · ${event.tool_name || 'unknown'}`,
            payload: JSON.stringify(event.type === 'tool_start' ? event.args : event.result, null, 2),
            body: event.error?.message,
            at: event.at,
          },
        ],
      };
    case 'done':
    case 'error':
      return applyTerminalEvent(nextState, event.event_record, event.error?.message);
    default:
      return nextState;
  }
}

export function applyTerminalEvent(state: ConsoleChatState, record?: EventRecord, errorMessage?: string): ConsoleChatState {
  if (!record) {
    return {
      ...state,
      statusLabel: errorMessage ? '运行失败' : '完成',
      errorText: errorMessage,
    };
  }

  return {
    ...state,
    sessionKey: record.session_key || state.sessionKey,
    statusLabel: record.status === 'failed' ? '运行失败' : '运行完成',
    errorText: record.error?.message,
    messages: state.messages.map((item) =>
      item.id === state.assistantMessageID
        ? { ...item, content: record.assistant_reply || item.content, streaming: false }
        : item,
    ),
    debugEntries: [
      ...state.debugEntries,
      {
        id: `terminal-${record.event_id}`,
        kind: record.status === 'failed' ? 'error' : 'done',
        title: record.status === 'failed' ? '终态 · 失败' : '终态 · 完成',
        body: record.error?.message || record.assistant_reply,
        at: record.updated_at,
      },
    ],
  };
}
