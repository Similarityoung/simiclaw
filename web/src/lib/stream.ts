import {
  CHAT_STREAM_PROTOCOL_VERSION,
  type ChatStreamEvent,
  type EventRecord,
  type RuntimeClient,
} from '../types';

const terminalEventTypes = new Set(['done', 'error']);

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
  readonly causeError: unknown;

  constructor(eventID: string, causeError: unknown) {
    super(`stream interrupted for ${eventID}`);
    this.name = 'StreamRecoverableError';
    this.eventID = eventID;
    this.causeError = causeError;
  }
}

class StopStreaming extends Error {
  constructor() {
    super('stop_streaming');
  }
}

export interface ParsedSSEEvent {
  eventType: string;
  data: string;
}

export async function readSSE(
  stream: ReadableStream<Uint8Array>,
  onEvent: (event: ParsedSSEEvent) => void | Promise<void>,
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
    const nextEvent = { eventType, data: dataLines.join('\n') };
    eventType = '';
    dataLines = [];
    await onEvent(nextEvent);
  };

  while (true) {
    const { done, value } = await reader.read();
    if (done) {
      buffer += decoder.decode();
      if (buffer.length > 0) {
        const normalized = buffer.replace(/\r/g, '');
        const lines = normalized.split('\n');
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
      }
      if (buffer.trim() !== '') {
        const line = buffer.replace(/\r/g, '');
        if (line.startsWith('event: ')) {
          eventType = line.slice('event: '.length).trim();
        } else if (line.startsWith('data: ')) {
          dataLines.push(line.slice('data: '.length));
        }
      }
      await flush();
      return;
    }

    buffer += decoder.decode(value, { stream: true });
    const normalized = buffer.replace(/\r/g, '');
    const lines = normalized.split('\n');
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
  }
}

export function isTerminalEvent(event: ChatStreamEvent): boolean {
  return terminalEventTypes.has(event.type);
}

export function isTerminalRecord(record: EventRecord): boolean {
  switch (record.status) {
    case 'suppressed':
    case 'failed':
      return true;
    case 'processed':
      return (
        !record.outbox_status ||
        record.outbox_status === 'sent' ||
        record.outbox_status === 'dead'
      );
    default:
      return false;
  }
}

export function eventFromRecord(record: EventRecord): ChatStreamEvent {
  return {
    type: record.status === 'failed' ? 'error' : 'done',
    event_id: record.event_id,
    sequence: 0,
    at: record.updated_at,
    event_record: record,
    error: record.error,
  };
}

export async function waitForTerminalEvent(
  client: Pick<RuntimeClient, 'getEvent'>,
  eventID: string,
  signal?: AbortSignal,
): Promise<EventRecord> {
  const deadline = Date.now() + 30000;
  while (Date.now() < deadline) {
    signal?.throwIfAborted();
    const record = await client.getEvent(eventID);
    if (isTerminalRecord(record)) {
      return record;
    }
    await sleep(700, signal);
  }
  throw new Error(`poll timeout after waiting for ${eventID}`);
}

export async function consumeChatStream(
  response: Response,
  client: Pick<RuntimeClient, 'getEvent'>,
  onEvent?: (event: ChatStreamEvent) => void | Promise<void>,
  signal?: AbortSignal,
): Promise<EventRecord> {
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
    await readSSE(response.body, async ({ eventType, data }) => {
      signal?.throwIfAborted();
      if (!data.trim()) {
        return;
      }
      const event = JSON.parse(data) as ChatStreamEvent;
      if (eventType && eventType !== event.type) {
        throw new Error(`stream event type mismatch: header=${eventType} payload=${event.type}`);
      }
      if (event.type === 'accepted') {
        acceptedEventID = event.event_id;
        await onEvent?.(event);
        if (event.stream_protocol_version !== CHAT_STREAM_PROTOCOL_VERSION) {
          throw new StreamRecoverableError(acceptedEventID, new Error('stream protocol mismatch'));
        }
        return;
      }

      await onEvent?.(event);
      if (!isTerminalEvent(event)) {
        return;
      }
      if (event.event_record) {
        terminalRecord = isTerminalRecord(event.event_record)
          ? event.event_record
          : await waitForTerminalEvent(client, event.event_record.event_id, signal);
        if (!isTerminalRecord(event.event_record)) {
          await onEvent?.(eventFromRecord(terminalRecord));
        }
        throw new StopStreaming();
      }
      if (event.error) {
        throw new APIError(200, event.error.message, event.error.code);
      }
      throw new Error('stream terminal event missing event_record');
    });
  } catch (error) {
    if (error instanceof StopStreaming && terminalRecord) {
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
}

function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = window.setTimeout(() => {
      cleanup();
      resolve();
    }, ms);
    const onAbort = () => {
      cleanup();
      reject(signal?.reason instanceof Error ? signal.reason : new DOMException('Aborted', 'AbortError'));
    };
    const cleanup = () => {
      window.clearTimeout(timer);
      signal?.removeEventListener('abort', onAbort);
    };
    signal?.addEventListener('abort', onAbort, { once: true });
  });
}
