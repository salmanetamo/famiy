import { useQuery } from '@tanstack/react-query';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, request } from './api/client';
import { App } from './App';
import { useLogin } from './auth/session';
import { sessionKey } from './auth/sessionKey';
import {
  alice,
  errorResponse,
  jsonResponse,
  renderWithClient,
  stubFetch,
  type FetchHandler,
} from './test-utils';
import { emptyGraph } from './test/fixtures';

/** stubFetch, plus an empty account for `GET /graph` (the signed-in screen always asks for it). */
const stubApi = (handler: FetchHandler) =>
  stubFetch((call) =>
    call.path === '/api/graph' ? jsonResponse(200, emptyGraph()) : handler(call),
  );

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('App session flow', () => {
  it('shows the sign-in form when there is no session', async () => {
    stubApi(() => errorResponse(401, 'unauthenticated', 'authentication required'));
    renderWithClient(<App />);
    expect(screen.getByRole('status')).toHaveTextContent('Checking your session');
    expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeInTheDocument();
  });

  it('shows the account when a session exists, and signs out', async () => {
    const calls = stubApi((call) =>
      call.path === '/api/auth/logout'
        ? new Response(null, { status: 204 })
        : jsonResponse(200, alice),
    );
    renderWithClient(<App />);
    expect(await screen.findByText('Signed in as alice@example.com')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Sign out' }));
    expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeInTheDocument();
    expect(calls.map((c) => `${c.method} ${c.path}`)).toContain('POST /api/auth/logout');
  });

  it('offers a retry when the session check itself fails', async () => {
    let healthy = false;
    stubApi(() => (healthy ? jsonResponse(200, alice) : errorResponse(500, 'internal', 'boom')));
    renderWithClient(<App />);
    expect(await screen.findByText(/Something went wrong \(internal\)/)).toBeInTheDocument();
    healthy = true;
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByText('Signed in as alice@example.com')).toBeInTheDocument();
  });

  it('drops all cached data on sign out, keeps it when sign out fails', async () => {
    let logoutFails = true;
    stubApi((call) => {
      if (call.path === '/api/auth/logout') {
        return logoutFails
          ? errorResponse(500, 'internal', 'boom')
          : new Response(null, { status: 204 });
      }
      return jsonResponse(200, alice);
    });
    const { client } = renderWithClient(<App />);
    await screen.findByText('Signed in as alice@example.com');
    client.setQueryData(['other-data'], { persons: [{ id: 1 }] });

    await userEvent.click(screen.getByRole('button', { name: 'Sign out' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Something went wrong (internal)');
    expect(screen.getByText('Signed in as alice@example.com')).toBeInTheDocument();
    expect(client.getQueryData(['other-data'])).toBeDefined();

    logoutFails = false;
    await userEvent.click(screen.getByRole('button', { name: 'Sign out' }));
    await screen.findByRole('heading', { name: 'Sign in' });
    expect(client.getQueryData(['other-data'])).toBeUndefined();
  });
});

function Probe() {
  const graph = useQuery({
    queryKey: ['probe'],
    queryFn: () => request<unknown>('GET', '/probe'),
  });
  return <p>{graph.isError ? 'graph failed' : 'graph pending'}</p>;
}

function LoginProbe() {
  const login = useLogin();
  return (
    <button
      type="button"
      onClick={() => {
        login.mutate({ email: 'a@example.com', password: 'wrong password here' });
      }}
    >
      try login
    </button>
  );
}

describe('401 handling', () => {
  it('a 401 unauthenticated from any request ends the session', async () => {
    stubApi((call) =>
      call.path === '/api/probe'
        ? errorResponse(401, 'unauthenticated', 'authentication required')
        : jsonResponse(200, alice),
    );
    const { client } = renderWithClient(
      <>
        <App />
        <Probe />
      </>,
    );
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Sign in' })).toBeInTheDocument();
    });
    expect(client.getQueryData(sessionKey)).toBeNull();
  });

  it('a 401 invalid_credentials from a login attempt does not end an existing session', async () => {
    stubApi((call) =>
      call.path === '/api/auth/login'
        ? errorResponse(401, 'invalid_credentials')
        : jsonResponse(200, alice),
    );
    const { client } = renderWithClient(
      <>
        <App />
        <LoginProbe />
      </>,
    );
    await screen.findByText('Signed in as alice@example.com');
    await userEvent.click(screen.getByRole('button', { name: 'try login' }));
    await waitFor(() => {
      expect(vi.mocked(fetch)).toHaveBeenCalledWith('/api/auth/login', expect.anything());
    });
    expect(client.getQueryData(sessionKey)).toEqual(alice);
    expect(screen.getByText('Signed in as alice@example.com')).toBeInTheDocument();
  });
});

describe('session lifecycle', () => {
  it('a 401 from any request also drops the other cached data', async () => {
    stubApi((call) =>
      call.path === '/api/probe'
        ? errorResponse(401, 'unauthenticated', 'authentication required')
        : jsonResponse(200, alice),
    );
    const { client } = renderWithClient(
      <>
        <App />
        <Probe />
      </>,
    );
    client.setQueryData(['other'], { secret: 1 });
    await screen.findByRole('heading', { name: 'Sign in' });
    expect(client.getQueryData(['other'])).toBeUndefined();
  });

  it('a response that arrives after sign out is discarded, not cached', async () => {
    stubApi((call) =>
      call.path === '/api/auth/logout'
        ? new Response(null, { status: 204 })
        : jsonResponse(200, alice),
    );
    const { client } = renderWithClient(<App />);
    await screen.findByText('Signed in as alice@example.com');
    let finish: (value: unknown) => void = () => undefined;
    const slow = client
      .query({
        queryKey: ['slow'],
        queryFn: () =>
          new Promise((resolve) => {
            finish = resolve;
          }),
      })
      .catch(() => undefined);
    await userEvent.click(screen.getByRole('button', { name: 'Sign out' }));
    await screen.findByRole('heading', { name: 'Sign in' });
    finish({ secret: 1 });
    await slow;
    expect(client.getQueryData(['slow'])).toBeUndefined();
  });

  it('signing in clears leftover data and ignores a late 401 from before the sign-in', async () => {
    stubApi((call) =>
      call.path === '/api/auth/login'
        ? jsonResponse(200, alice)
        : errorResponse(401, 'unauthenticated', 'authentication required'),
    );
    const { client } = renderWithClient(
      <>
        <App />
        <LoginProbe />
      </>,
    );
    await screen.findByRole('heading', { name: 'Sign in' });
    client.setQueryData(['stale'], { from: 'previous account' });
    let fail: (error: unknown) => void = () => undefined;
    const old = client
      .query({
        queryKey: ['before-sign-in'],
        queryFn: () =>
          new Promise((_resolve, reject) => {
            fail = reject;
          }),
      })
      .catch(() => undefined);

    await userEvent.click(screen.getByRole('button', { name: 'try login' }));
    await waitFor(() => {
      expect(client.getQueryData(sessionKey)).toEqual(alice);
    });
    expect(client.getQueryData(['stale'])).toBeUndefined();

    fail(new ApiError({ status: 401, code: 'unauthenticated', message: 'late' }));
    await old;
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(client.getQueryData(sessionKey)).toEqual(alice);
  });
});

describe('focus and live regions', () => {
  it('moves focus to the page heading after signing out, and leaves it alone on first load', async () => {
    stubApi((call) =>
      call.path === '/api/auth/logout'
        ? new Response(null, { status: 204 })
        : jsonResponse(200, alice),
    );
    renderWithClient(<App />);
    await screen.findByText('Signed in as alice@example.com');
    expect(document.body).toHaveFocus();
    await userEvent.click(screen.getByRole('button', { name: 'Sign out' }));
    await screen.findByRole('heading', { name: 'Sign in' });
    expect(screen.getByRole('heading', { name: 'Family Graph' })).toHaveFocus();
  });

  it('moves focus to the page heading after signing in', async () => {
    stubApi((call) =>
      call.path === '/api/auth/login'
        ? jsonResponse(200, alice)
        : errorResponse(401, 'unauthenticated', 'authentication required'),
    );
    renderWithClient(<App />);
    await screen.findByRole('heading', { name: 'Sign in' });
    await userEvent.type(screen.getByLabelText('Email'), 'alice@example.com');
    await userEvent.type(screen.getByLabelText('Password'), 'correct horse battery');
    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }));
    await screen.findByText('Signed in as alice@example.com');
    expect(screen.getByRole('heading', { name: 'Family Graph' })).toHaveFocus();
  });

  it('keeps one live region from loading to error, so the error is announced', async () => {
    stubApi(() => errorResponse(500, 'internal', 'boom'));
    renderWithClient(<App />);
    const region = screen.getByRole('status');
    expect(region).toHaveTextContent('Checking your session');
    await screen.findByText(/Something went wrong \(internal\)/);
    expect(screen.getByRole('status')).toBe(region);
  });
});

describe('sign-in racing the first session check', () => {
  it('a slow "no session" answer cannot overwrite a sign-in that finished first', async () => {
    let answerMe: (res: Response) => void = () => undefined;
    stubApi((call) =>
      call.path === '/api/auth/login'
        ? jsonResponse(200, alice)
        : new Promise<Response>((resolve) => {
            answerMe = resolve;
          }),
    );
    const { client } = renderWithClient(
      <>
        <App />
        <LoginProbe />
      </>,
    );
    expect(screen.getByRole('status')).toHaveTextContent('Checking your session');
    await userEvent.click(screen.getByRole('button', { name: 'try login' }));
    await waitFor(() => {
      expect(client.getQueryData(sessionKey)).toEqual(alice);
    });

    answerMe(errorResponse(401, 'unauthenticated', 'authentication required'));
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(client.getQueryData(sessionKey)).toEqual(alice);
    expect(await screen.findByText('Signed in as alice@example.com')).toBeInTheDocument();
  });
});
