// The only module that calls fetch. Every failure becomes an ApiError carrying the server's
// stable error code, so callers branch on `code`, never on message text.

/** All API calls go through this prefix: Vite proxies it in development (see vite.config.ts). */
export const API_BASE = '/api';

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly retryAfterSeconds: number | undefined;
  readonly requestId: string | undefined;

  constructor(init: {
    status: number;
    code: string;
    message: string;
    retryAfterSeconds?: number | undefined;
    requestId?: string | undefined;
  }) {
    super(init.message);
    this.name = 'ApiError';
    this.status = init.status;
    this.code = init.code;
    this.retryAfterSeconds = init.retryAfterSeconds;
    this.requestId = init.requestId;
  }
}

type Method = 'GET' | 'POST' | 'PATCH' | 'DELETE';

interface ErrorBody {
  error: { code: string; message: string };
}

function isErrorBody(value: unknown): value is ErrorBody {
  if (typeof value !== 'object' || value === null || !('error' in value)) return false;
  const inner = value.error;
  return (
    typeof inner === 'object' &&
    inner !== null &&
    'code' in inner &&
    'message' in inner &&
    typeof inner.code === 'string' &&
    typeof inner.message === 'string'
  );
}

function parseRetryAfter(header: string | null): number | undefined {
  if (header === null) return undefined;
  const seconds = Number.parseInt(header, 10);
  return Number.isFinite(seconds) && seconds >= 0 ? seconds : undefined;
}

async function readJson(res: Response): Promise<unknown> {
  try {
    const parsed: unknown = await res.json();
    return parsed;
  } catch {
    return undefined;
  }
}

/**
 * Sends one request. `T` is the caller's claim about the success body, taken from the generated
 * OpenAPI types; the body is not validated at runtime (the API is first-party and its contract
 * is covered by the backend's tests). Use `void` for 204 responses.
 */
export async function request<T>(method: Method, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  const init: RequestInit = { method, headers };
  if (body !== undefined) {
    // The API answers 415 to an unsafe request with a body that is not application/json.
    headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(body);
  }

  let res: Response;
  try {
    res = await fetch(`${API_BASE}${path}`, init);
  } catch {
    throw new ApiError({
      status: 0,
      code: 'network_error',
      message: 'The server could not be reached.',
    });
  }

  const requestId = res.headers.get('X-Request-Id') ?? undefined;

  if (!res.ok) {
    const payload = await readJson(res);
    if (isErrorBody(payload)) {
      throw new ApiError({
        status: res.status,
        code: payload.error.code,
        message: payload.error.message,
        retryAfterSeconds: parseRetryAfter(res.headers.get('Retry-After')),
        requestId,
      });
    }
    throw new ApiError({
      status: res.status,
      code: 'unexpected_response',
      message: `Unexpected response (HTTP ${String(res.status)}).`,
      requestId,
    });
  }

  if (res.status === 204) return undefined as T;

  const payload = await readJson(res);
  if (payload === undefined) {
    throw new ApiError({
      status: res.status,
      code: 'unexpected_response',
      message: 'The server sent a response that is not valid JSON.',
      requestId,
    });
  }
  return payload as T;
}
