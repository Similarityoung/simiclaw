import type { ChatMessageItem, ChatStreamEvent, DebugEntry, EventRecord, MessageRecord } from '../types';

export interface LiveRunState {
  messages: ChatMessageItem[];
  debugEntries: DebugEntry[];
  assistantMessageID?: string;
  sessionKey?: string;
  sessionID?: string;
  statusLabel: string;
  errorText?: string;
}

export function createInitialRunState(messages: ChatMessageItem[] = []): LiveRunState {
  return {
    messages,
    debugEntries: [],
    statusLabel: '等待输入',
  };
}

export function mapHistoryMessage(item: MessageRecord): ChatMessageItem | null {
  if (!item.visible) {
    return null;
  }
  if (item.role !== 'user' && item.role !== 'assistant') {
    return null;
  }
  return {
    id: item.message_id,
    role: item.role,
    content: item.content,
    createdAt: item.created_at,
  };
}

export function appendUserMessage(state: LiveRunState, text: string): LiveRunState {
  return {
    ...state,
    statusLabel: '请求发送中',
    errorText: undefined,
    messages: [
      ...state.messages,
      {
        id: `local-user-${Date.now()}`,
        role: 'user',
        content: text,
        createdAt: new Date().toISOString(),
      },
    ],
  };
}

export function applyStreamEvent(state: LiveRunState, event: ChatStreamEvent): LiveRunState {
  let nextState = {
    ...state,
    sessionKey: event.ingest_response?.session_key || event.event_record?.session_key || state.sessionKey,
    sessionID:
      event.ingest_response?.active_session_id || event.event_record?.session_id || state.sessionID,
  };

  switch (event.type) {
    case 'accepted':
      nextState.statusLabel = '请求已接收';
      nextState.debugEntries = [
        ...nextState.debugEntries,
        createDebugEntry(event, 'accepted', '请求已接收', `event_id: ${event.event_id}`),
      ];
      return nextState;
    case 'status': {
      const statusText = event.message?.trim() || event.status?.trim() || '处理中';
      nextState.statusLabel = statusText;
      const lastEntry = nextState.debugEntries[nextState.debugEntries.length - 1];
      if (lastEntry?.kind === 'status' && lastEntry.body === statusText) {
        return nextState;
      }
      nextState.debugEntries = [
        ...nextState.debugEntries,
        createDebugEntry(event, 'status', '状态更新', statusText),
      ];
      return nextState;
    }
    case 'reasoning_delta': {
      nextState.statusLabel = '模型思考中';
      const delta = event.delta;
      if (delta === undefined || delta === '') {
        return nextState;
      }
      const entries = [...nextState.debugEntries];
      const lastEntry = entries[entries.length - 1];
      if (lastEntry?.kind === 'reasoning_delta') {
        lastEntry.body = `${lastEntry.body ?? ''}${delta}`;
        nextState.debugEntries = entries;
        return nextState;
      }
      nextState.debugEntries = [
        ...entries,
        createDebugEntry(event, 'reasoning_delta', 'Reasoning', delta),
      ];
      return nextState;
    }
    case 'text_delta': {
      nextState.statusLabel = '回复生成中';
      const delta = event.delta ?? '';
      if (!delta) {
        return nextState;
      }
      const messages = [...nextState.messages];
      if (!nextState.assistantMessageID) {
        const messageID = `local-assistant-${Date.now()}`;
        messages.push({
          id: messageID,
          role: 'assistant',
          content: delta,
          createdAt: event.at,
          streaming: true,
        });
        nextState.messages = messages;
        nextState.assistantMessageID = messageID;
        return nextState;
      }
      nextState.messages = messages.map((item) =>
        item.id === nextState.assistantMessageID
          ? { ...item, content: `${item.content}${delta}`, streaming: true }
          : item,
      );
      return nextState;
    }
    case 'tool_start':
      nextState.statusLabel = `调用工具 ${event.tool_name || event.tool_call_id || 'unknown'}`;
      nextState.debugEntries = [
        ...nextState.debugEntries,
        createDebugEntry(
          event,
          'tool_start',
          `工具开始 · ${event.tool_name || event.tool_call_id || 'unknown'}`,
          undefined,
          formatPayload(event.args, event.truncated),
        ),
      ];
      return nextState;
    case 'tool_result':
      nextState.statusLabel = event.error ? '工具执行失败' : '工具执行完成';
      nextState.debugEntries = [
        ...nextState.debugEntries,
        createDebugEntry(
          event,
          'tool_result',
          `工具结果 · ${event.tool_name || event.tool_call_id || 'unknown'}`,
          event.error ? `${event.error.code}: ${event.error.message}` : undefined,
          event.error ? undefined : formatPayload(event.result, event.truncated),
        ),
      ];
      return nextState;
    case 'done':
    case 'error':
      return applyTerminalRecord(nextState, event.event_record, event.error);
    default:
      return nextState;
  }
}

export function applyTerminalRecord(
  state: LiveRunState,
  record?: EventRecord,
  error?: { code: string; message: string },
): LiveRunState {
  const nextState = { ...state };

  if (!record) {
    nextState.statusLabel = error ? '运行失败' : '完成';
    nextState.errorText = error ? `${error.code}: ${error.message}` : undefined;
    nextState.debugEntries = [
      ...nextState.debugEntries,
      {
        id: `debug-terminal-${Date.now()}`,
        kind: error ? 'error' : 'done',
        title: error ? '运行失败' : '运行完成',
        body: error ? `${error.code}: ${error.message}` : '终态已提交',
        at: new Date().toISOString(),
        tone: error ? 'danger' : 'success',
      },
    ];
    return nextState;
  }

  nextState.sessionKey = record.session_key || nextState.sessionKey;
  nextState.sessionID = record.session_id || nextState.sessionID;
  nextState.statusLabel = record.status === 'failed' ? '运行失败' : '运行完成';
  nextState.errorText = record.error ? `${record.error.code}: ${record.error.message}` : undefined;
  nextState.messages = finalizeAssistantMessage(nextState.messages, nextState.assistantMessageID, record);
  nextState.debugEntries = [
    ...nextState.debugEntries,
    {
      id: `debug-terminal-${record.event_id}`,
      kind: record.status === 'failed' ? 'error' : 'done',
      title: record.status === 'failed' ? '终态 · 失败' : '终态 · 完成',
      body:
        record.status === 'failed'
          ? record.error?.message || '事件进入 failed'
          : `event_id: ${record.event_id}`,
      payload: record.assistant_reply || undefined,
      at: record.updated_at,
      tone: record.status === 'failed' ? 'danger' : 'success',
    },
  ];
  return nextState;
}

function finalizeAssistantMessage(
  messages: ChatMessageItem[],
  assistantMessageID: string | undefined,
  record: EventRecord,
): ChatMessageItem[] {
  if (record.status === 'failed') {
    return messages.map((item) =>
      item.id === assistantMessageID ? { ...item, streaming: false } : item,
    );
  }
  if (!record.assistant_reply) {
    return messages.map((item) =>
      item.id === assistantMessageID ? { ...item, streaming: false } : item,
    );
  }
  if (!assistantMessageID) {
    return [
      ...messages,
      {
        id: `assistant-final-${record.event_id}`,
        role: 'assistant',
        content: record.assistant_reply,
        createdAt: record.updated_at,
      },
    ];
  }
  return messages.map((item) =>
    item.id === assistantMessageID
      ? { ...item, content: record.assistant_reply || item.content, streaming: false }
      : item,
  );
}

function createDebugEntry(
  event: ChatStreamEvent,
  kind: DebugEntry['kind'],
  title: string,
  body?: string,
  payload?: string,
): DebugEntry {
  return {
    id: `debug-${kind}-${event.sequence}-${event.at}`,
    kind,
    title,
    body,
    payload,
    at: event.at,
    tone:
      kind === 'error'
        ? 'danger'
        : kind === 'done'
          ? 'success'
          : kind === 'tool_result' && event.error
            ? 'danger'
            : 'neutral',
  };
}

function formatPayload(payload?: Record<string, unknown>, truncated?: boolean): string | undefined {
  if (!payload || Object.keys(payload).length === 0) {
    return truncated ? '{}\n[truncated]' : undefined;
  }
  const formatted = JSON.stringify(payload, null, 2);
  return truncated ? `${formatted}\n[truncated]` : formatted;
}
