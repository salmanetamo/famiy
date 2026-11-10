import { screen, waitFor } from '@testing-library/react';
import type { ApiGraph } from '../api/types';
import { GraphScreen } from '../graph/GraphScreen';
import { createQueryClient } from '../queryClient';
import { alice, renderWithClient, stubFetch } from '../test-utils';
import { fakeServer, type Stored } from './fakeServer';
import { apiGraphFrom, messy } from './fixtures';
import { createInlineEngine } from './inlineEngine';

const engine = createInlineEngine();

/** Renders the whole graph screen against a fake server that remembers what is written. */
export function setupGraph(
  family: Parameters<typeof apiGraphFrom>[0] = messy,
  details: Record<number, Partial<Stored>> = {},
  api?: ApiGraph,
) {
  const { api: built, idOf } = apiGraphFrom(family);
  const server = fakeServer(api ?? built, details);
  const calls = stubFetch(server.handler);
  const client = createQueryClient();
  client.setQueryData(['session'], alice);
  renderWithClient(<GraphScreen getLayoutEngine={() => engine} />, client);
  const writes = () => calls.filter((c) => c.method !== 'GET');
  return { idOf, server, calls, writes, client };
}

export const people = () => [...document.querySelectorAll<SVGGElement>('[data-person-id]')];

export const node = (id: number) => {
  const found = document.querySelector<SVGGElement>(`[data-person-id="${String(id)}"]`);
  if (!found) throw new Error(`no node for ${String(id)}`);
  return found;
};

export async function waitForPeople(count: number) {
  await waitFor(() => {
    expect(people()).toHaveLength(count);
  });
}

export const panel = (name: string | RegExp) => screen.getByRole('region', { name });
