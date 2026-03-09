import type {
  ErrorResponse,
  EventRecord,
  IngestRequest,
  MessagePage,
  RuntimeClient,
  SendChatOptions,
  SessionHistoryParams,
  SessionPage,
  SessionRecord,
} from '../types';
import { APIError, StreamRecoverableError, consumeChatStream, eventFromRecord, waitForTerminalEvent } from './stream';

const configuredBaseURL = (import.meta.env.VITE_API_BASE_URL as string | undefined)?.trim() || '';

function apiURL(path: string): string {
  if (!configuredBaseURL) {
    return path;
  }
  return new URL(path, configuredBaseURL).toString();
}

async function decodeJSON<T>(response: Response): Promise<T> {
  if (!response.ok) {
    throw await decodeAPIError(response);
  }
  return (await response.json()) as T;
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
  } catch {
  }
  return new APIError(response.status, text.trim());
}

function withQuery(path: string, query: URLSearchParams): string {
  const full = apiURL(path);
  const rendered = query.toString();
  if (!rendered) {
    return full;
  }
  return `${full}?${rendered}`;
}

export function createRuntimeClient(fetcher: typeof fetch = fetch): RuntimeClient {
  return {
    async listSessions(params = {}): Promise<SessionPage> {
      const query = new URLSearchParams();
      if (params.cursor) {
        query.set('cursor', params.cursor);
      }
      if (params.limit) {
        query.set('limit', String(params.limit));
      }
      const response = await fetcher(withQuery('/v1/sessions', query));
      return decodeJSON<SessionPage>(response);
    },

    async getSession(sessionKey: string): Promise<SessionRecord> {
      const response = await fetcher(apiURL(`/v1/sessions/${encodeURIComponent(sessionKey)}`));
      return decodeJSON<SessionRecord>(response);
    },

    async getSessionHistory(params: SessionHistoryParams): Promise<MessagePage> {
      const query = new URLSearchParams();
      if (params.cursor) {
        query.set('cursor', params.cursor);
      }
      if (params.limit) {
        query.set('limit', String(params.limit));
      }
      if (params.visibleOnly === false) {
        query.set('visible', 'false');
      }
      const response = await fetcher(
        withQuery(`/v1/sessions/${encodeURIComponent(params.sessionKey)}/history`, query),
      );
      return decodeJSON<MessagePage>(response);
    },

    async getEvent(eventID: string): Promise<EventRecord> {
      const response = await fetcher(apiURL(`/v1/events/${encodeURIComponent(eventID)}`));
      return decodeJSON<EventRecord>(response);
    },

    async sendChat(request: IngestRequest, options?: SendChatOptions): Promise<EventRecord> {
      const response = await fetcher(apiURL('/v1/chat:stream'), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(request),
        signal: options?.signal,
      });

      if (!response.ok) {
        throw await decodeAPIError(response);
      }

      try {
        return await consumeChatStream(response, this, options?.onEvent, options?.signal);
      } catch (error) {
        if (error instanceof StreamRecoverableError && error.eventID) {
          const record = await waitForTerminalEvent(this, error.eventID, options?.signal);
          await options?.onEvent?.(eventFromRecord(record));
          return record;
        }
        throw error;
      }
    },
  };
}

const runtimeClient = createRuntimeClient();

export default runtimeClient;
