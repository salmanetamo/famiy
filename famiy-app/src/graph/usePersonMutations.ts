import { useMutation, useQueryClient } from '@tanstack/react-query';
import { createPerson, deletePerson, updatePerson } from '../api/persons';
import type { CreatePersonBody, PatchPersonBody } from '../api/types';
import { refreshGraph, refreshIfStale } from './refresh';
import { personKey } from './useFamilyGraph';

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
    onError: (error) => refreshIfStale(client, error),
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
    onError: (error) => refreshIfStale(client, error),
  });
}
