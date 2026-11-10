import { QueryClientProvider, type QueryClient } from '@tanstack/react-query';
import { render } from '@testing-library/react';
import type { ReactElement } from 'react';
import { vi } from 'vitest';
import { createQueryClient } from './queryClient';

export function jsonResponse(
  status: number,
  body?: unknown,
  headers: Record<string, string> = {},
): Response {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json', ...headers },
  });
}

export function errorResponse(
  status: number,
  code: string,
  message = code,
  headers: Record<string, string> = {},
): Response {
  return jsonResponse(status, { error: { code, message } }, headers);
}

export interface FetchCall {
  method: string;
  path: string;
  headers: Record<string, string>;
  body: string | undefined;
}

type Handler = (call: FetchCall) => Response | Promise<Response>;

/** Replaces global fetch with `handler`; returns the recorded calls. Removed by vi.unstubAllGlobals. */
export function stubFetch(handler: Handler): FetchCall[] {
  const calls: FetchCall[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const call: FetchCall = {
        method: init?.method ?? 'GET',
        path: url,
        headers: Object.fromEntries(new Headers(init?.headers).entries()),
        body: typeof init?.body === 'string' ? init.body : undefined,
      };
      calls.push(call);
      return Promise.resolve(handler(call));
    }),
  );
  return calls;
}

export const alice = { id: 1, email: 'alice@example.com', created_at: '2026-11-01T10:00:00Z' };

export function renderWithClient(ui: ReactElement, client: QueryClient = createQueryClient()) {
  return {
    client,
    ...render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>),
  };
}
