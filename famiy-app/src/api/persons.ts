import { request } from './client';
import type { PersonDetail } from './types';

/** One person with the fields the graph leaves out (bio, residence). */
export function getPerson(id: number): Promise<PersonDetail> {
  return request<PersonDetail>('GET', `/persons/${String(id)}`);
}
