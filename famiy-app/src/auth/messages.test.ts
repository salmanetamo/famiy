import { describe, expect, it } from 'vitest';
import { ApiError } from '../api/client';
import { authErrorMessage } from './messages';

const err = (code: string, extra: { retryAfterSeconds?: number; requestId?: string } = {}) =>
  new ApiError({ status: 400, code, message: `server says ${code}`, ...extra });

describe('authErrorMessage', () => {
  it.each([
    ['network_error', /could not reach the server/i],
    ['invalid_credentials', /email or password is incorrect/i],
    ['registration_closed', /registration is closed/i],
    ['email_taken', /already exists/i],
  ])('maps %s to plain wording', (code, pattern) => {
    expect(authErrorMessage(err(code))).toMatch(pattern);
  });

  it('shows the server message for validation_failed', () => {
    expect(authErrorMessage(err('validation_failed'))).toBe('server says validation_failed');
  });

  it('rounds the wait up to whole minutes, with singular and plural', () => {
    expect(authErrorMessage(err('too_many_attempts', { retryAfterSeconds: 900 }))).toMatch(
      /try again in 15 minutes\./i,
    );
    expect(authErrorMessage(err('too_many_attempts', { retryAfterSeconds: 61 }))).toMatch(
      /try again in 2 minutes\./i,
    );
    expect(authErrorMessage(err('too_many_registrations', { retryAfterSeconds: 30 }))).toMatch(
      /try again in 1 minute\./i,
    );
  });

  it('says "try again later" when there is no Retry-After', () => {
    expect(authErrorMessage(err('too_many_attempts'))).toMatch(/try again later/i);
  });

  it('gives the code and the request reference for an unknown error', () => {
    expect(authErrorMessage(err('weird', { requestId: 'req-1' }))).toBe(
      'Something went wrong (weird). Reference: req-1.',
    );
  });

  it('does not leak a non-API error message', () => {
    expect(authErrorMessage(new Error('secret internals'))).toBe(
      'Something went wrong. Please try again.',
    );
  });
});
