import type { QueryClient } from '@tanstack/react-query';
import { ApiError } from '../api/client';
import { graphKey } from './useFamilyGraph';

/**
 * There is no delta endpoint, so after any write the whole graph is fetched again. The mutation
 * stays pending until that fetch is done, so "saving" lasts until the screen can show the result.
 * A failed refetch does not fail the write (it did succeed); the screen shows its own refresh alert.
 */
export async function refreshGraph(client: QueryClient): Promise<void> {
  await client.refetchQueries({ queryKey: graphKey });
}

const STALE_CODES = new Set(['person_not_found', 'parent_link_not_found', 'partnership_not_found']);

/**
 * A person or link the screen still shows is gone (changed elsewhere), so what is on screen is stale:
 * fetch the tree again. Other refusals (a rule, a limit) say nothing about the data being out of date.
 */
export async function refreshIfStale(client: QueryClient, error: unknown): Promise<void> {
  if (error instanceof ApiError && STALE_CODES.has(error.code)) await refreshGraph(client);
}
