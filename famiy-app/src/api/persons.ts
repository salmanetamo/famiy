import { request } from './client';
import type { CreatePersonBody, PatchPersonBody, PersonDetail } from './types';

/** One person with the fields the graph leaves out (bio, residence). */
export function getPerson(id: number): Promise<PersonDetail> {
  return request<PersonDetail>('GET', `/persons/${String(id)}`);
}

export function createPerson(body: CreatePersonBody): Promise<PersonDetail> {
  return request<PersonDetail>('POST', '/persons', body);
}

/** Merge patch: only the fields present are changed, and `null` clears an optional one. */
export function updatePerson(id: number, patch: PatchPersonBody): Promise<PersonDetail> {
  return request<PersonDetail>('PATCH', `/persons/${String(id)}`, patch);
}

/** Permanent. The server also removes the person's parent links and partnerships. */
export function deletePerson(id: number): Promise<void> {
  return request<undefined>('DELETE', `/persons/${String(id)}`);
}
