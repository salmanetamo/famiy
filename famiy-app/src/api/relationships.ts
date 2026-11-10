import { request } from './client';
import type { PartnerKind } from './types';

/** Links `parentId` as a parent of `childId`. A child has at most two parents and no cycles. */
export function addParent(childId: number, parentId: number): Promise<unknown> {
  return request<unknown>('POST', `/persons/${String(childId)}/parents`, { parent_id: parentId });
}

export function removeParent(childId: number, parentId: number): Promise<undefined> {
  return request<undefined>('DELETE', `/persons/${String(childId)}/parents/${String(parentId)}`);
}

/** Records a partnership. The order of the two people does not matter to the server. */
export function addPartner(id: number, partnerId: number, kind: PartnerKind): Promise<unknown> {
  return request<unknown>('POST', `/persons/${String(id)}/partners`, {
    partner_id: partnerId,
    kind,
  });
}

export function updatePartner(id: number, partnerId: number, kind: PartnerKind): Promise<unknown> {
  return request<unknown>('PATCH', `/persons/${String(id)}/partners/${String(partnerId)}`, {
    kind,
  });
}

export function removePartner(id: number, partnerId: number): Promise<undefined> {
  return request<undefined>('DELETE', `/persons/${String(id)}/partners/${String(partnerId)}`);
}
