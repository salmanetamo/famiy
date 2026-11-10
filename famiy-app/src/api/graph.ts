import { request } from './client';
import type { ApiGraph } from './types';

/** The whole account in one consistent snapshot (people, parent links, partnerships). */
export function getGraph(): Promise<ApiGraph> {
  return request<ApiGraph>('GET', '/graph');
}
