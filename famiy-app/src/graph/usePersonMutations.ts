import { useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query';
import { ApiError } from '../api/client';
import { createPerson, deletePerson, updatePerson } from '../api/persons';
import type { CreatePersonBody, PatchPersonBody } from '../api/types';
import { graphKey, personKey } from './useFamilyGraph';

/**
 * There is no delta endpoint, so after any write the whole graph is fetched again. The mutation
 * stays pending until that fetch is done, so "saving" lasts until the screen can show the result.
 * A failed refetch does not fail the write (it did succeed); the screen shows its own refresh alert.
 */
async function refreshGraph(client: QueryClient): Promise<void> {
  await client.refetchQueries({ queryKey: graphKey });
}

/** The person is gone (deleted elsewhere): what is on screen is stale, so fetch the tree again. */
async function refreshIfPersonIsGone(client: QueryClient, error: unknown): Promise<void> {
  if (error instanceof ApiError && error.code === 'person_not_found') await refreshGraph(client);
}

export function useCreatePerson() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (body: CreatePersonBody) => createPerson(body),
    onSuccess: () => refreshGraph(client),
  });
}

export function useUpdatePerson(id: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (patch: PatchPersonBody) => updatePerson(id, patch),
    onSuccess: async (person) => {
      client.setQueryData(personKey(id), person);
      await refreshGraph(client);
    },
    onError: (error) => refreshIfPersonIsGone(client, error),
  });
}

export function useDeletePerson() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => deletePerson(id),
    onSuccess: async (_result, id) => {
      // Refresh first: once the person is gone from the graph their panel unmounts, so nothing
      // refetches the deleted record in the meantime.
      await refreshGraph(client);
      client.removeQueries({ queryKey: personKey(id) });
    },
    onError: (error) => refreshIfPersonIsGone(client, error),
  });
}
