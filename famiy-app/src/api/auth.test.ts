import { afterEach, describe, expect, it, vi } from 'vitest';
import { me } from './auth';
import { ApiError } from './client';
import { alice, errorResponse, jsonResponse, stubFetch } from '../test-utils';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('me', () => {
  it('returns the account when signed in', async () => {
    stubFetch(() => jsonResponse(200, alice));
    await expect(me()).resolves.toEqual(alice);
  });

  it('returns null for unauthenticated', async () => {
    stubFetch(() => errorResponse(401, 'unauthenticated', 'authentication required'));
    await expect(me()).resolves.toBeNull();
  });

  it('still throws for any other failure', async () => {
    stubFetch(() => errorResponse(500, 'internal', 'boom'));
    await expect(me()).rejects.toBeInstanceOf(ApiError);
  });

  it('throws for a 401 with a different code (not a plain "no session")', async () => {
    stubFetch(() => errorResponse(401, 'invalid_credentials'));
    await expect(me()).rejects.toBeInstanceOf(ApiError);
  });
});
