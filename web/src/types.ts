export const CHAT_STREAM_PROTOCOL_VERSION = '2026-03-07.sse.v1';

export type EventStatus =
  | 'received'
  | 'queued'
  | 'processing'
  | 'processed'
  | 'suppressed'
  | 'failed';

export type RunMode = 'NORMAL' | 'NO_REPLY';

export type OutboxStatus = 'pending' | 'sending' | 'sent' | 'retry_wait' | 'dead';

export type ChatStreamEventType =
  | 'accepted'
  | 'status'
  | 'reasoning_delta'
  | 'text_delta'
  | 'tool_start'
  | 'tool_result'
  | 'done'
  | 'error';

export interface Conversation {
  conversation_id: string;
  thread_id?: string;
  channel_type: string;
  participant_id?: string;
}

export interface EventPayload {
  type: string;
  text?: string;
}

export interface IngestRequest {
  source: string;
  conversation: Conversation;
  session_key?: string;
  idempotency_key: string;
  timestamp: string;
  payload: EventPayload;
}

export interface IngestResponse {
  event_id: string;
  session_key: string;
  active_session_id: string;
  received_at: string;
  payload_hash: string;
}

export interface ErrorBlock {
  code: string;
  message: string;
  details?: Record<string, unknown>;
}

export interface ErrorResponse {
  error: ErrorBlock;
}

export interface MessageRecord {
  message_id: string;
  session_key: string;
  session_id: string;
  run_id: string;
  role: string;
  content: string;
  visible: boolean;
  tool_call_id?: string;
  tool_name?: string;
  tool_args?: Record<string, unknown>;
  tool_result?: Record<string, unknown>;
  meta?: Record<string, unknown>;
  created_at: string;
}

export interface EventRecord {
  event_id: string;
  status: EventStatus;
  outbox_status?: OutboxStatus;
  session_key: string;
  session_id: string;
  run_id?: string;
  run_mode?: RunMode;
  assistant_reply?: string;
  outbox_id?: string;
  processing_started_at?: string;
  received_at: string;
  created_at: string;
  updated_at: string;
  payload_hash: string;
  provider?: string;
  model?: string;
  provider_request_id?: string;
  error?: ErrorBlock;
}

export interface SessionRecord {
  session_key: string;
  active_session_id: string;
  conversation_id?: string;
  channel_type?: string;
  participant_id?: string;
  dm_scope?: string;
  message_count: number;
  prompt_tokens_total: number;
  completion_tokens_total: number;
  total_tokens_total: number;
  last_model?: string;
  last_run_id?: string;
  last_activity_at: string;
  created_at: string;
  updated_at: string;
}

export interface SessionPage {
  items: SessionRecord[];
  next_cursor?: string;
}

export interface MessagePage {
  items: MessageRecord[];
  next_cursor?: string;
}

export interface ChatStreamEvent {
  type: ChatStreamEventType;
  event_id: string;
  sequence: number;
  at: string;
  stream_protocol_version?: string;
  status?: string;
  message?: string;
  delta?: string;
  tool_call_id?: string;
  tool_name?: string;
  args?: Record<string, unknown>;
  result?: Record<string, unknown>;
  truncated?: boolean;
  ingest_response?: IngestResponse;
  event_record?: EventRecord;
  error?: ErrorBlock;
}

export interface ChatMessageItem {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  createdAt?: string;
  streaming?: boolean;
}

export interface DebugEntry {
  id: string;
  kind: ChatStreamEventType;
  title: string;
  body?: string;
  payload?: string;
  at?: string;
  tone: 'neutral' | 'success' | 'warning' | 'danger';
}

export interface ListSessionsParams {
  cursor?: string;
  limit?: number;
}

export interface SessionHistoryParams {
  sessionKey: string;
  cursor?: string;
  limit?: number;
  visibleOnly?: boolean;
}

export interface SendChatOptions {
  onEvent?: (event: ChatStreamEvent) => void | Promise<void>;
  signal?: AbortSignal;
}

export interface RuntimeClient {
  listSessions(params?: ListSessionsParams): Promise<SessionPage>;
  getSession(sessionKey: string): Promise<SessionRecord>;
  getSessionHistory(params: SessionHistoryParams): Promise<MessagePage>;
  getEvent(eventID: string): Promise<EventRecord>;
  sendChat(request: IngestRequest, options?: SendChatOptions): Promise<EventRecord>;
}
