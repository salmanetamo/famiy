import { useMutation, useQueryClient } from '@tanstack/react-query';
import {
  addParent,
  addPartner,
  removeParent,
  removePartner,
  updatePartner,
} from '../api/relationships';
import type { PartnerKind } from '../api/types';
import { refreshGraph, refreshIfStale } from './refresh';

/**
 * Every relationship write has the same shape: send it, fetch the graph again, and stay pending
 * until that is done. If the server says a person or link is gone, the screen is stale, so fetch too.
 */
function useWrite<Variables>(write: (variables: Variables) => Promise<unknown>) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: write,
    onSuccess: () => refreshGraph(client),
    onError: (error) => refreshIfStale(client, error),
  });
}

export interface ParentLink {
  childId: number;
  parentId: number;
}

export interface PartnerLink {
  id: number;
  partnerId: number;
}

export const useAddParent = () =>
  useWrite(({ childId, parentId }: ParentLink) => addParent(childId, parentId));

export const useRemoveParent = () =>
  useWrite(({ childId, parentId }: ParentLink) => removeParent(childId, parentId));

export const useAddPartner = () =>
  useWrite(({ id, partnerId, kind }: PartnerLink & { kind: PartnerKind }) =>
    addPartner(id, partnerId, kind),
  );

export const useUpdatePartner = () =>
  useWrite(({ id, partnerId, kind }: PartnerLink & { kind: PartnerKind }) =>
    updatePartner(id, partnerId, kind),
  );

export const useRemovePartner = () =>
  useWrite(({ id, partnerId }: PartnerLink) => removePartner(id, partnerId));
