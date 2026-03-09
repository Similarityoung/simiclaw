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
const configuredAPIKey = (import.meta.env.VITE_API_KEY as string | undefined)?.trim() || '';

interface RuntimeClientConfig {
  baseURL?: string;
  apiKey?: string;
}

function apiURL(path: string, baseURL: string): string {
  if (!baseURL) {
    return path;
  }
  const normalizedBaseURL = baseURL.endsWith('/') ? baseURL : `${baseURL}/`;
  const normalizedPath = path.startsWith('/') ? path.slice(1) : path;
  return new URL(normalizedPath, normalizedBaseURL).toString();
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

function withQuery(path: string, query: URLSearchParams, baseURL: string): string {
  const full = apiURL(path, baseURL);
  const rendered = query.toString();
  if (!rendered) {
    return full;
  }
  return `${full}?${rendered}`;
}

function authHeaders(apiKey: string, extra: HeadersInit = {}): HeadersInit {
  if (!apiKey) {
    return extra;
  }
  return {
    ...extra,
    Authorization: `Bearer ${apiKey}`,
  };
}

export function createRuntimeClient(fetcher: typeof fetch = fetch, config: RuntimeClientConfig = {}): RuntimeClient {
  const baseURL = typeof config.baseURL === 'string' ? config.baseURL.trim() : configuredBaseURL;
  const apiKey = typeof config.apiKey === 'string' ? config.apiKey.trim() : configuredAPIKey;

  return {
    async listSessions(params = {}): Promise<SessionPage> {
      const query = new URLSearchParams();
      if (params.cursor) {
        query.set('cursor', params.cursor);
      }
      if (params.limit) {
        query.set('limit', String(params.limit));
      }
      const response = await fetcher(withQuery('/v1/sessions', query, baseURL), {
        headers: authHeaders(apiKey),
      });
      return decodeJSON<SessionPage>(response);
    },

    async getSession(sessionKey: string): Promise<SessionRecord> {
      const response = await fetcher(apiURL(`/v1/sessions/${encodeURIComponent(sessionKey)}`, baseURL), {
        headers: authHeaders(apiKey),
      });
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
        withQuery(`/v1/sessions/${encodeURIComponent(params.sessionKey)}/history`, query, baseURL),
        {
          headers: authHeaders(apiKey),
        },
      );
      return decodeJSON<MessagePage>(response);
    },

    async getEvent(eventID: string): Promise<EventRecord> {
      const response = await fetcher(apiURL(`/v1/events/${encodeURIComponent(eventID)}`, baseURL), {
        headers: authHeaders(apiKey),
      });
      return decodeJSON<EventRecord>(response);
    },

    async sendChat(request: IngestRequest, options?: SendChatOptions): Promise<EventRecord> {
      const response = await fetcher(apiURL('/v1/chat:stream', baseURL), {
        method: 'POST',
        headers: authHeaders(apiKey, {
          'Content-Type': 'application/json',
        }),
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
