import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, request } from './client';
import { errorResponse, jsonResponse, stubFetch } from '../test-utils';

afterEach(() => {
  vi.unstubAllGlobals();
});

async function caught(promise: Promise<unknown>): Promise<ApiError> {
  try {
    await promise;
  } catch (error) {
    if (error instanceof ApiError) return error;
    throw error;
  }
  throw new Error('expected the request to fail');
}

describe('request', () => {
  it('GETs through the /api prefix and returns the parsed body, without a Content-Type', async () => {
    const calls = stubFetch(() => jsonResponse(200, { id: 7 }));
    await expect(request<{ id: number }>('GET', '/persons/7')).resolves.toEqual({ id: 7 });
    expect(calls).toHaveLength(1);
    expect(calls[0]?.path).toBe('/api/persons/7');
    expect(calls[0]?.method).toBe('GET');
    expect(calls[0]?.headers['content-type']).toBeUndefined();
    expect(calls[0]?.headers.accept).toBe('application/json');
  });

  it('sends a JSON body with Content-Type application/json', async () => {
    const calls = stubFetch(() => jsonResponse(201, { id: 1 }));
    await request('POST', '/persons', { given_name: 'Ada' });
    expect(calls[0]?.headers['content-type']).toBe('application/json');
    expect(calls[0]?.body).toBe('{"given_name":"Ada"}');
  });

  it('sends no body and no Content-Type for a POST without one', async () => {
    const calls = stubFetch(() => new Response(null, { status: 204 }));
    await request<undefined>('POST', '/auth/logout');
    expect(calls[0]?.body).toBeUndefined();
    expect(calls[0]?.headers['content-type']).toBeUndefined();
  });

  it('resolves undefined for a 204', async () => {
    stubFetch(() => new Response(null, { status: 204 }));
    await expect(request<undefined>('DELETE', '/persons/1')).resolves.toBeUndefined();
  });

  it('turns an error body into an ApiError with the server code and request id', async () => {
    stubFetch(() =>
      errorResponse(409, 'duplicate_parent', 'already a parent', { 'X-Request-Id': 'req-9' }),
    );
    const error = await caught(request('POST', '/persons/1/parents', { parent_id: 2 }));
    expect(error.status).toBe(409);
    expect(error.code).toBe('duplicate_parent');
    expect(error.message).toBe('already a parent');
    expect(error.requestId).toBe('req-9');
    expect(error.retryAfterSeconds).toBeUndefined();
  });

  it('reads Retry-After from a 429', async () => {
    stubFetch(() => errorResponse(429, 'too_many_attempts', 'slow down', { 'Retry-After': '900' }));
    const error = await caught(request('POST', '/auth/login', {}));
    expect(error.retryAfterSeconds).toBe(900);
  });

  it('ignores a Retry-After that is not a number', async () => {
    stubFetch(() =>
      errorResponse(429, 'too_many_attempts', 'slow down', { 'Retry-After': 'soon' }),
    );
    const error = await caught(request('POST', '/auth/login', {}));
    expect(error.retryAfterSeconds).toBeUndefined();
  });

  it.each([502, 503, 504])(
    'reports a %i with no API error body (a proxy in front of a down server) as server_unavailable',
    async (status) => {
      stubFetch(() => new Response('Bad Gateway', { status }));
      const error = await caught(request('GET', '/graph'));
      expect(error.code).toBe('server_unavailable');
      expect(error.status).toBe(status);
    },
  );

  it('reports any other non-JSON error as unexpected_response', async () => {
    stubFetch(() => new Response('oops', { status: 500 }));
    const error = await caught(request('GET', '/graph'));
    expect(error.code).toBe('unexpected_response');
  });

  it('keeps the code from the API itself when a 503 does carry an error body', async () => {
    stubFetch(() => errorResponse(503, 'unavailable', 'database down'));
    const error = await caught(request('GET', '/healthz'));
    expect(error.code).toBe('unavailable');
  });

  it('reports a JSON error that lacks the error envelope as unexpected_response', async () => {
    stubFetch(() => jsonResponse(500, { message: 'boom' }));
    const error = await caught(request('GET', '/graph'));
    expect(error.code).toBe('unexpected_response');
  });

  it('reports a 200 whose body is not JSON as unexpected_response', async () => {
    stubFetch(() => new Response('<html>', { status: 200 }));
    const error = await caught(request('GET', '/graph'));
    expect(error.code).toBe('unexpected_response');
    expect(error.status).toBe(200);
  });

  it('reports a failed fetch as network_error with status 0', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')));
    const error = await caught(request('GET', '/graph'));
    expect(error.code).toBe('network_error');
    expect(error.status).toBe(0);
  });
});
