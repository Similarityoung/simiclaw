export interface ErrorBlock {
  code: string;
  message: string;
  details?: Record<string, unknown>;
}

export interface ErrorResponse {
  error: ErrorBlock;
}

export type EventStatus = 'received' | 'queued' | 'processing' | 'processed' | 'suppressed' | 'failed';
export type RunStatus = 'started' | 'succeeded' | 'failed' | 'cancelled' | string;
export type RunMode = 'NORMAL' | 'NO_REPLY';
export type OutboxStatus = 'pending' | 'sending' | 'sent' | 'retry_wait' | 'dead';

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

export interface SessionPage {
  items: SessionRecord[];
  next_cursor?: string;
}

export interface MessagePage {
  items: MessageRecord[];
  next_cursor?: string;
}

export interface RunSummary {
  run_id: string;
  event_id: string;
  session_key: string;
  session_id: string;
  run_mode: RunMode;
  status: RunStatus;
  started_at: string;
  ended_at: string;
}

export interface RunPage {
  items: RunSummary[];
  next_cursor?: string;
}

export interface ToolCall {
  tool_call_id: string;
  name: string;
  args?: Record<string, unknown>;
}

export interface ToolExecution {
  tool_call_id?: string;
  tool_name?: string;
  status?: string;
  started_at?: string;
  finished_at?: string;
  error?: ErrorBlock;
  result_preview?: string;
  args?: Record<string, unknown>;
  result?: Record<string, unknown>;
}

export interface RunTrace {
  run_id: string;
  event_id: string;
  session_key: string;
  session_id: string;
  run_mode: RunMode;
  status: RunStatus;
  started_at: string;
  finished_at: string;
  provider?: string;
  model?: string;
  prompt_tokens?: number;
  completion_tokens?: number;
  total_tokens?: number;
  latency_ms?: number;
  finish_reason?: string;
  raw_finish_reason?: string;
  provider_request_id?: string;
  output_text?: string;
  tool_calls?: ToolCall[];
  tool_executions?: ToolExecution[];
  diagnostics?: Record<string, string>;
  error?: ErrorBlock;
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
  received_at: string;
  created_at: string;
  updated_at: string;
  payload_hash: string;
  provider?: string;
  model?: string;
  provider_request_id?: string;
  error?: ErrorBlock;
}

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

export type ChatStreamEventType =
  | 'accepted'
  | 'status'
  | 'reasoning_delta'
  | 'text_delta'
  | 'tool_start'
  | 'tool_result'
  | 'done'
  | 'error';

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

export interface ListRunsParams {
  sessionKey?: string;
  cursor?: string;
  limit?: number;
}

export interface SendChatOptions {
  onEvent?: (event: ChatStreamEvent) => void | Promise<void>;
  signal?: AbortSignal;
}

export class APIError extends Error {
  readonly statusCode: number;
  readonly code?: string;

  constructor(statusCode: number, message: string, code?: string) {
    super(message);
    this.name = 'APIError';
    this.statusCode = statusCode;
    this.code = code;
  }
}

export class StreamRecoverableError extends Error {
  readonly eventID: string;

  constructor(eventID: string, causeError: unknown) {
    super(`stream interrupted for ${eventID}`);
    this.name = 'StreamRecoverableError';
    this.eventID = eventID;
    this.cause = causeError;
  }
}

const configuredBaseURL = (import.meta.env.VITE_API_BASE_URL as string | undefined)?.trim() || '';
const chatStreamProtocolVersion = '2026-03-07.sse.v1';
const terminalPollTimeoutMs = 60000;

function normalizeBaseURL(baseURL: string): string {
  const trimmed = baseURL.trim();
  if (!trimmed) {
    return '';
  }

  if (/^[a-zA-Z][a-zA-Z\d+.-]*:/.test(trimmed)) {
    const parsed = new URL(trimmed);
    if (typeof window !== 'undefined' && parsed.origin !== window.location.origin) {
      throw new Error('cross-origin VITE_API_BASE_URL unsupported; use same-origin path prefix or Vite dev proxy');
    }
    return `${parsed.origin}${normalizePathPrefix(parsed.pathname)}`;
  }

  return normalizePathPrefix(trimmed);
}

function normalizePathPrefix(pathPrefix: string): string {
  const normalized = pathPrefix.trim().replace(/^\/+/, '').replace(/\/+$/, '');
  return normalized ? `/${normalized}` : '';
}

function apiURL(path: string, baseURL: string): string {
  const normalizedPath = path.startsWith('/') ? path : `/${path}`;
  const normalizedBaseURL = normalizeBaseURL(baseURL);
  return normalizedBaseURL ? `${normalizedBaseURL}${normalizedPath}` : normalizedPath;
}

function withQuery(path: string, query: URLSearchParams, baseURL: string): string {
  const full = apiURL(path, baseURL);
  const rendered = query.toString();
  return rendered ? `${full}?${rendered}` : full;
}

async function decodeAPIError(response: Response): Promise<APIError> {
  const text = await response.text();
  if (!text.trim()) {
    return new APIError(response.status, `http status ${response.status}`);
  }

  try {
    const parsed = JSON.parse(text) as ErrorResponse;
    if (parsed.error?.code) {
      return new APIError(response.status, parsed.error.message, parsed.error.code);
    }
  } catch {}

  return new APIError(response.status, text.trim());
}

async function decodeJSON<T>(response: Response): Promise<T> {
  if (!response.ok) {
    throw await decodeAPIError(response);
  }
  return (await response.json()) as T;
}

async function readSSE(
  stream: ReadableStream<Uint8Array>,
  onEvent: (eventType: string, data: string) => void | Promise<void>,
): Promise<void> {
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let eventType = '';
  let dataLines: string[] = [];

  const flush = async () => {
    if (!eventType && dataLines.length === 0) {
      return;
    }
    const nextData = dataLines.join('\n');
    const nextEventType = eventType;
    eventType = '';
    dataLines = [];
    await onEvent(nextEventType, nextData);
  };

  while (true) {
    const { done, value } = await reader.read();
    buffer += done ? decoder.decode() : decoder.decode(value, { stream: true });
    const lines = buffer.replace(/\r/g, '').split('\n');
    buffer = lines.pop() ?? '';

    for (const line of lines) {
      if (line === '') {
        await flush();
        continue;
      }
      if (line.startsWith(':')) {
        continue;
      }
      if (line.startsWith('event: ')) {
        eventType = line.slice('event: '.length).trim();
        continue;
      }
      if (line.startsWith('data: ')) {
        dataLines.push(line.slice('data: '.length));
      }
    }

    if (done) {
      await flush();
      return;
    }
  }
}

function isTerminalRecord(record: EventRecord): boolean {
  switch (record.status) {
    case 'suppressed':
    case 'failed':
      return true;
    case 'processed':
      return !record.outbox_status || record.outbox_status === 'sent' || record.outbox_status === 'dead';
    default:
      return false;
  }
}

function eventFromRecord(record: EventRecord): ChatStreamEvent {
  return {
    type: record.status === 'failed' ? 'error' : 'done',
    event_id: record.event_id,
    sequence: 0,
    at: record.updated_at,
    event_record: record,
    error: record.error,
  };
}

function isStreamFallbackStatus(status: number): boolean {
  return status === 404 || status === 405 || status === 501 || status === 502;
}

async function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  await new Promise<void>((resolve, reject) => {
    const timer = window.setTimeout(() => {
      signal?.removeEventListener('abort', onAbort);
      resolve();
    }, ms);

    const onAbort = () => {
      window.clearTimeout(timer);
      reject(signal?.reason instanceof Error ? signal.reason : new DOMException('Aborted', 'AbortError'));
    };

    signal?.addEventListener('abort', onAbort, { once: true });
  });
}

export interface RuntimeClient {
  getHealth(): Promise<Record<string, unknown>>;
  getReady(): Promise<Record<string, unknown>>;
  listSessions(params?: ListSessionsParams): Promise<SessionPage>;
  getSession(sessionKey: string): Promise<SessionRecord>;
  getSessionHistory(params: SessionHistoryParams): Promise<MessagePage>;
  listRuns(params?: ListRunsParams): Promise<RunPage>;
  getRunTrace(runID: string): Promise<RunTrace>;
  getEvent(eventID: string): Promise<EventRecord>;
  sendChat(request: IngestRequest, options?: SendChatOptions): Promise<EventRecord>;
}

export function createRuntimeClient(fetcher: typeof fetch = fetch, config: { baseURL?: string } = {}): RuntimeClient {
  const baseURL = typeof config.baseURL === 'string' ? config.baseURL.trim() : configuredBaseURL;

  const waitForTerminalEvent = async (eventID: string, signal?: AbortSignal): Promise<EventRecord> => {
    const deadline = Date.now() + terminalPollTimeoutMs;
    while (Date.now() < deadline) {
      signal?.throwIfAborted();
      const record = await client.getEvent(eventID);
      if (isTerminalRecord(record)) {
        return record;
      }
      await sleep(700, signal);
    }
    throw new Error(`poll timeout after waiting for ${eventID}`);
  };

  const consumeChatStream = async (response: Response, options?: SendChatOptions): Promise<EventRecord> => {
    if (!response.body) {
      throw new Error('streaming unsupported');
    }

    const contentType = response.headers.get('content-type')?.toLowerCase() ?? '';
    if (!contentType.startsWith('text/event-stream')) {
      throw new Error('streaming unsupported');
    }

    let acceptedEventID = '';
    let terminalRecord: EventRecord | undefined;

    try {
      await readSSE(response.body, async (eventType, data) => {
        options?.signal?.throwIfAborted();
        if (!data.trim()) {
          return;
        }

        const event = JSON.parse(data) as ChatStreamEvent;
        if (eventType && eventType !== event.type) {
          throw new Error(`stream event type mismatch: header=${eventType} payload=${event.type}`);
        }

        if (event.type === 'accepted') {
          acceptedEventID = event.event_id;
          await options?.onEvent?.(event);
          if (event.stream_protocol_version !== chatStreamProtocolVersion) {
            throw new StreamRecoverableError(acceptedEventID, new Error('stream protocol mismatch'));
          }
          return;
        }

        await options?.onEvent?.(event);
        if (event.type !== 'done' && event.type !== 'error') {
          return;
        }

        if (event.event_record) {
          terminalRecord = isTerminalRecord(event.event_record)
            ? event.event_record
            : await waitForTerminalEvent(event.event_record.event_id, options?.signal);
          if (!isTerminalRecord(event.event_record)) {
            await options?.onEvent?.(eventFromRecord(terminalRecord));
          }
          throw new Error('__STOP_STREAM__');
        }

        if (event.error) {
          throw new APIError(200, event.error.message, event.error.code);
        }

        throw new Error('stream terminal event missing event_record');
      });
    } catch (error) {
      if (error instanceof Error && error.message === '__STOP_STREAM__' && terminalRecord) {
        return terminalRecord;
      }
      if (error instanceof StreamRecoverableError) {
        throw error;
      }
      if (acceptedEventID) {
        throw new StreamRecoverableError(acceptedEventID, error);
      }
      throw error;
    }

    if (terminalRecord) {
      return terminalRecord;
    }
    if (acceptedEventID) {
      throw new StreamRecoverableError(acceptedEventID, new Error('stream closed before terminal event'));
    }
    throw new Error('stream finished without accepted event');
  };

  const ingestAndWait = async (request: IngestRequest, options?: SendChatOptions): Promise<EventRecord> => {
    const ingestResponse = await decodeJSON<IngestResponse>(
      await fetcher(apiURL('/v1/events:ingest', baseURL), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(request),
        signal: options?.signal,
      }),
    );
    await options?.onEvent?.({
      type: 'accepted',
      event_id: ingestResponse.event_id,
      sequence: 1,
      at: ingestResponse.received_at || new Date().toISOString(),
      stream_protocol_version: chatStreamProtocolVersion,
      ingest_response: ingestResponse,
    });
    const record = await waitForTerminalEvent(ingestResponse.event_id, options?.signal);
    await options?.onEvent?.(eventFromRecord(record));
    return record;
  };

  const client: RuntimeClient = {
    async getHealth() {
      return decodeJSON<Record<string, unknown>>(await fetcher(apiURL('/healthz', baseURL)));
    },
    async getReady() {
      return decodeJSON<Record<string, unknown>>(await fetcher(apiURL('/readyz', baseURL)));
    },
    async listSessions(params = {}) {
      const query = new URLSearchParams();
      if (params.cursor) query.set('cursor', params.cursor);
      if (params.limit) query.set('limit', String(params.limit));
      return decodeJSON<SessionPage>(await fetcher(withQuery('/v1/sessions', query, baseURL)));
    },
    async getSession(sessionKey) {
      return decodeJSON<SessionRecord>(await fetcher(apiURL(`/v1/sessions/${encodeURIComponent(sessionKey)}`, baseURL)));
    },
    async getSessionHistory(params) {
      const query = new URLSearchParams();
      if (params.cursor) query.set('cursor', params.cursor);
      if (params.limit) query.set('limit', String(params.limit));
      if (params.visibleOnly === false) query.set('visible', 'false');
      return decodeJSON<MessagePage>(
        await fetcher(withQuery(`/v1/sessions/${encodeURIComponent(params.sessionKey)}/history`, query, baseURL)),
      );
    },
    async listRuns(params = {}) {
      const query = new URLSearchParams();
      if (params.sessionKey) query.set('session_key', params.sessionKey);
      if (params.cursor) query.set('cursor', params.cursor);
      if (params.limit) query.set('limit', String(params.limit));
      return decodeJSON<RunPage>(await fetcher(withQuery('/v1/runs', query, baseURL)));
    },
    async getRunTrace(runID) {
      return decodeJSON<RunTrace>(await fetcher(apiURL(`/v1/runs/${encodeURIComponent(runID)}/trace`, baseURL)));
    },
    async getEvent(eventID) {
      return decodeJSON<EventRecord>(await fetcher(apiURL(`/v1/events/${encodeURIComponent(eventID)}`, baseURL)));
    },
    async sendChat(request, options) {
      const response = await fetcher(apiURL('/v1/chat:stream', baseURL), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(request),
        signal: options?.signal,
      });

      if (!response.ok) {
        if (isStreamFallbackStatus(response.status)) {
          return ingestAndWait(request, options);
        }
        throw await decodeAPIError(response);
      }

      try {
        return await consumeChatStream(response, options);
      } catch (error) {
        if (error instanceof Error && error.message === 'streaming unsupported') {
          return ingestAndWait(request, options);
        }
        if (error instanceof StreamRecoverableError && error.eventID) {
          const record = await waitForTerminalEvent(error.eventID, options?.signal);
          await options?.onEvent?.(eventFromRecord(record));
          return record;
        }
        throw error;
      }
    },
  };

  return client;
}

export const runtimeClient = createRuntimeClient((input, init) => fetch(input, init));

export function toErrorMessage(error: unknown): string {
  if (error instanceof APIError) {
    return error.code ? `${error.code}: ${error.message}` : error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return '未知错误';
}
