import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AuthPage } from './AuthPage';
import { sessionKey } from './sessionKey';
import { alice, errorResponse, jsonResponse, renderWithClient, stubFetch } from '../test-utils';

afterEach(() => {
  vi.unstubAllGlobals();
});

async function fillAndSubmit(email: string, password: string, button: string) {
  const user = userEvent.setup();
  await user.type(screen.getByLabelText('Email'), email);
  await user.type(screen.getByLabelText('Password'), password);
  await user.click(screen.getByRole('button', { name: button }));
}

describe('AuthPage', () => {
  it('has labelled fields and the right autocomplete hints for signing in', () => {
    renderWithClient(<AuthPage />);
    expect(screen.getByRole('heading', { name: 'Sign in' })).toBeInTheDocument();
    expect(screen.getByLabelText('Email')).toHaveAttribute('autocomplete', 'username');
    expect(screen.getByLabelText('Password')).toHaveAttribute('autocomplete', 'current-password');
    expect(screen.getByRole('alert')).toBeEmptyDOMElement();
  });

  it('signs in: posts trimmed credentials as JSON and stores the account as the session', async () => {
    const calls = stubFetch(() => jsonResponse(200, alice));
    const { client } = renderWithClient(<AuthPage />);
    await fillAndSubmit('  alice@example.com ', 'correct horse battery', 'Sign in');
    await waitFor(() => {
      expect(client.getQueryData(sessionKey)).toEqual(alice);
    });
    expect(calls[0]?.path).toBe('/api/auth/login');
    expect(calls[0]?.headers['content-type']).toBe('application/json');
    expect(JSON.parse(calls[0]?.body ?? '{}')).toEqual({
      email: 'alice@example.com',
      password: 'correct horse battery',
    });
  });

  it('announces a wrong password and clears the password field', async () => {
    stubFetch(() => errorResponse(401, 'invalid_credentials'));
    const { client } = renderWithClient(<AuthPage />);
    await fillAndSubmit('alice@example.com', 'wrong password here', 'Sign in');
    expect(await screen.findByRole('alert')).toHaveTextContent('Email or password is incorrect.');
    expect(screen.getByLabelText('Password')).toHaveValue('');
    expect(screen.getByLabelText('Email')).toHaveValue('alice@example.com');
    expect(client.getQueryData(sessionKey)).toBeUndefined();
  });

  it('tells the user how long to wait after a 429', async () => {
    stubFetch(() => errorResponse(429, 'too_many_attempts', 'x', { 'Retry-After': '900' }));
    renderWithClient(<AuthPage />);
    await fillAndSubmit('alice@example.com', 'whatever password', 'Sign in');
    expect(await screen.findByRole('alert')).toHaveTextContent('Try again in 15 minutes.');
  });

  it('disables the button while the request is pending', async () => {
    let release: (res: Response) => void = () => undefined;
    stubFetch(() => new Promise<Response>((resolve) => (release = resolve)));
    renderWithClient(<AuthPage />);
    await fillAndSubmit('alice@example.com', 'whatever password', 'Sign in');
    expect(await screen.findByRole('button', { name: 'Please wait…' })).toBeDisabled();
    release(errorResponse(401, 'invalid_credentials'));
    expect(await screen.findByRole('button', { name: 'Sign in' })).toBeEnabled();
  });

  it('switches to registration with new-password hints, and registers', async () => {
    const calls = stubFetch(() => jsonResponse(201, alice));
    const { client } = renderWithClient(<AuthPage />);
    await userEvent.click(screen.getByRole('button', { name: 'Create an account' }));
    expect(screen.getByRole('heading', { name: 'Create account' })).toBeInTheDocument();
    expect(screen.getByLabelText('Password')).toHaveAttribute('autocomplete', 'new-password');
    expect(screen.getByLabelText('Password')).toHaveAccessibleDescription('12 to 128 characters.');
    await fillAndSubmit('alice@example.com', 'correct horse battery', 'Create account');
    await waitFor(() => {
      expect(client.getQueryData(sessionKey)).toEqual(alice);
    });
    expect(calls[0]?.path).toBe('/api/auth/register');
  });

  it.each([
    [403, 'registration_closed', /registration is closed/i],
    [409, 'email_taken', /already exists/i],
  ])('explains a %i %s on registration', async (status, code, pattern) => {
    stubFetch(() => errorResponse(status, code));
    renderWithClient(<AuthPage />);
    await userEvent.click(screen.getByRole('button', { name: 'Create an account' }));
    await fillAndSubmit('alice@example.com', 'correct horse battery', 'Create account');
    expect(await screen.findByRole('alert')).toHaveTextContent(pattern);
  });

  it('clears an earlier error when switching mode', async () => {
    stubFetch(() => errorResponse(401, 'invalid_credentials'));
    renderWithClient(<AuthPage />);
    await fillAndSubmit('alice@example.com', 'wrong password here', 'Sign in');
    await screen.findByText('Email or password is incorrect.');
    await userEvent.click(screen.getByRole('button', { name: 'Create an account' }));
    expect(screen.getByRole('alert')).toBeEmptyDOMElement();
  });

  it('says the server could not be reached on a network failure', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')));
    renderWithClient(<AuthPage />);
    await fillAndSubmit('alice@example.com', 'whatever password', 'Sign in');
    expect(await screen.findByRole('alert')).toHaveTextContent(/could not reach the server/i);
  });
});
