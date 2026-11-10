import { useQuery } from '@tanstack/react-query';
import { getGraph } from '../api/graph';
import { buildFamilyGraph } from '../domain/family';

export const graphKey = ['graph'] as const;

/** The full record of one person (bio, residence, ...). */
export const personKey = (id: number) => ['person', id] as const;

/**
 * The account's family graph. After any change to people or links, refetch this query (there is
 * no delta endpoint). `select` is a stable function, so the normalised graph keeps one identity
 * until new data arrives, which is what lets layout run once per change.
 */
export function useFamilyGraph() {
  return useQuery({ queryKey: graphKey, queryFn: getGraph, select: buildFamilyGraph });
}
