import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from './client';
import { describeWriteError } from './describeError';
import { addParent, addPartner, removeParent, removePartner, updatePartner } from './relationships';
import { errorResponse, jsonResponse, stubFetch } from '../test-utils';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('relationship writes', () => {
  it('adds a parent: the child is in the path, the parent in the body', async () => {
    const calls = stubFetch(() => jsonResponse(201, { child_id: 9, parent_id: 7 }));
    await addParent(9, 7);
    expect(calls[0]).toMatchObject({ method: 'POST', path: '/api/persons/9/parents' });
    expect(JSON.parse(calls[0]?.body ?? '')).toEqual({ parent_id: 7 });
  });

  it('removes a parent link with DELETE and no body', async () => {
    const calls = stubFetch(() => new Response(null, { status: 204 }));
    await expect(removeParent(9, 7)).resolves.toBeUndefined();
    expect(calls[0]).toMatchObject({
      method: 'DELETE',
      path: '/api/persons/9/parents/7',
      body: undefined,
    });
  });

  it('adds a partner with the kind', async () => {
    const calls = stubFetch(() => jsonResponse(201, { partner_id: 7, kind: 'partnership' }));
    await addPartner(9, 7, 'partnership');
    expect(calls[0]).toMatchObject({ method: 'POST', path: '/api/persons/9/partners' });
    expect(JSON.parse(calls[0]?.body ?? '')).toEqual({ partner_id: 7, kind: 'partnership' });
  });

  it('changes the kind with PATCH', async () => {
    const calls = stubFetch(() => jsonResponse(200, { partner_id: 7, kind: 'other' }));
    await updatePartner(9, 7, 'other');
    expect(calls[0]).toMatchObject({ method: 'PATCH', path: '/api/persons/9/partners/7' });
    expect(JSON.parse(calls[0]?.body ?? '')).toEqual({ kind: 'other' });
  });

  it('removes a partnership with DELETE', async () => {
    const calls = stubFetch(() => new Response(null, { status: 204 }));
    await removePartner(9, 7);
    expect(calls[0]).toMatchObject({ method: 'DELETE', path: '/api/persons/9/partners/7' });
  });

  it('turns a refusal into an ApiError with the server code', async () => {
    stubFetch(() => errorResponse(409, 'too_many_parents'));
    await expect(addParent(9, 7)).rejects.toMatchObject({ status: 409, code: 'too_many_parents' });
  });
});

describe('describeWriteError for relationships', () => {
  const api = (code: string) => new ApiError({ status: 409, code, message: 'server text' });

  it.each([
    ['too_many_parents', /already has two parents/],
    ['duplicate_parent', /already recorded/],
    ['would_create_cycle', /their own ancestor/],
    ['duplicate_partnership', /already recorded as partners/],
    ['self_parent', /themselves/],
    ['self_partner', /themselves/],
    ['parent_link_not_found', /no longer exists/],
    ['partnership_not_found', /no longer exists/],
  ])('explains %s in plain words', (code, text) => {
    expect(describeWriteError(api(code))).toMatch(text);
  });
});
