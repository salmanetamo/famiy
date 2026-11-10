import { afterEach, describe, expect, it, vi } from 'vitest';
import { describeWriteError } from './describeError';
import { ApiError } from './client';
import { createPerson, deletePerson, updatePerson } from './persons';
import { errorResponse, jsonResponse, stubFetch } from '../test-utils';

afterEach(() => {
  vi.unstubAllGlobals();
});

const person = {
  id: 5,
  given_name: 'Nia',
  family_name: null,
  birth_date: null,
  birth_approx: false,
  death_date: null,
  bio: null,
  residence: null,
  created_at: '2026-11-01T10:00:00Z',
};

describe('person writes', () => {
  it('creates with POST /persons and a JSON body', async () => {
    const calls = stubFetch(() => jsonResponse(201, person));
    const body = {
      given_name: 'Nia',
      family_name: null,
      birth_date: null,
      birth_approx: false,
      death_date: null,
      bio: null,
      residence: null,
    };
    await expect(createPerson(body)).resolves.toEqual(person);
    expect(calls[0]).toMatchObject({ method: 'POST', path: '/api/persons' });
    expect(calls[0]?.headers['content-type']).toBe('application/json');
    expect(JSON.parse(calls[0]?.body ?? '')).toEqual(body);
  });

  it('updates with PATCH /persons/{id}, sending null to clear a field', async () => {
    const calls = stubFetch(() => jsonResponse(200, person));
    await updatePerson(5, { family_name: null });
    expect(calls[0]).toMatchObject({ method: 'PATCH', path: '/api/persons/5' });
    expect(calls[0]?.body).toBe('{"family_name":null}');
  });

  it('deletes with DELETE /persons/{id} and no body', async () => {
    const calls = stubFetch(() => new Response(null, { status: 204 }));
    await expect(deletePerson(5)).resolves.toBeUndefined();
    expect(calls[0]).toMatchObject({ method: 'DELETE', path: '/api/persons/5', body: undefined });
    expect(calls[0]?.headers['content-type']).toBeUndefined();
  });

  it('turns a refusal into an ApiError with the server code', async () => {
    stubFetch(() => errorResponse(400, 'validation_failed', 'given_name is required'));
    await expect(createPerson({ given_name: '', birth_approx: false })).rejects.toMatchObject({
      status: 400,
      code: 'validation_failed',
    });
  });
});

describe('describeWriteError', () => {
  const api = (code: string, message = 'server text') =>
    new ApiError({ status: 400, code, message });

  it("shows the server's own message for rules the user can act on", () => {
    expect(describeWriteError(api('validation_failed', 'birth_date is in the future'))).toBe(
      'birth_date is in the future',
    );
    expect(describeWriteError(api('person_limit_reached', 'this account is full'))).toBe(
      'this account is full',
    );
  });

  it('explains a person who no longer exists', () => {
    expect(describeWriteError(api('person_not_found'))).toMatch(/no longer exists/);
  });

  it('falls back to the general text for anything else, including a lost connection', () => {
    expect(describeWriteError(api('internal'))).toMatch(/Something went wrong \(internal\)/);
    expect(describeWriteError(api('network_error'))).toMatch(/Could not reach the server/);
    expect(describeWriteError(new Error('x'))).toMatch(/Something went wrong/);
  });
});
