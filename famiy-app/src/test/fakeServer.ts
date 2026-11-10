import type { ApiGraph } from '../api/types';
import { errorResponse, jsonResponse, type FetchCall } from '../test-utils';

export interface Stored {
  id: number;
  given_name: string;
  family_name: string | null;
  birth_date: string | null;
  birth_approx: boolean;
  death_date: string | null;
  bio: string | null;
  residence: string | null;
  created_at: string;
}

type Interceptor = (call: FetchCall) => Response | Promise<Response> | undefined;

/**
 * A small stand-in for the API that remembers what was written, so the refetch shows the result.
 * It applies the server's real relationship rules (two parents at most, no repeats, no cycles) so
 * tests can see what a refusal looks like, but the picker is expected to prevent most of them.
 */
export function fakeServer(start: ApiGraph, details: Record<number, Partial<Stored>> = {}) {
  const store = new Map<number, Stored>(
    start.persons.map((p) => [
      p.id,
      { ...p, bio: null, residence: null, created_at: '2026-11-01T10:00:00Z', ...details[p.id] },
    ]),
  );
  const links = start.parent_links.map((l) => ({ ...l }));
  const partnerships = start.partnerships.map((p) => ({ partners: [...p.partners], kind: p.kind }));
  let nextId = 1000;
  /** Return a Response to override a matching request; return undefined to let it through. */
  const intercept: { current: Interceptor | null } = { current: null };

  const summary = (p: Stored) => ({
    id: p.id,
    given_name: p.given_name,
    family_name: p.family_name,
    birth_date: p.birth_date,
    birth_approx: p.birth_approx,
    death_date: p.death_date,
  });
  const body = (call: FetchCall): Record<string, unknown> =>
    JSON.parse(call.body ?? '{}') as Record<string, unknown>;
  const text = (value: unknown, fallback: string): string =>
    typeof value === 'string' ? value : fallback;
  const isDescendant = (ancestor: number, candidate: number): boolean => {
    const queue = [ancestor];
    const seen = new Set<number>();
    for (let current = queue.pop(); current !== undefined; current = queue.pop()) {
      for (const link of links) {
        if (link.parent_id === current && !seen.has(link.child_id)) {
          if (link.child_id === candidate) return true;
          seen.add(link.child_id);
          queue.push(link.child_id);
        }
      }
    }
    return false;
  };
  const findPartnership = (a: number, b: number) =>
    partnerships.find((p) => p.partners.includes(a) && p.partners.includes(b));

  const handler = (call: FetchCall): Response | Promise<Response> => {
    const overridden = intercept.current?.(call);
    if (overridden) return overridden;
    const path = call.path.replace(/^\/api/, '');

    if (call.method === 'GET' && path === '/graph') {
      return jsonResponse(200, {
        persons: [...store.values()].map(summary),
        parent_links: links.filter((l) => store.has(l.parent_id) && store.has(l.child_id)),
        partnerships: partnerships.filter((p) => p.partners.every((x) => store.has(x))),
      });
    }
    if (call.method === 'POST' && path === '/persons') {
      const input = body(call) as Partial<Stored>;
      const created: Stored = {
        id: nextId++,
        given_name: input.given_name ?? '',
        family_name: input.family_name ?? null,
        birth_date: input.birth_date ?? null,
        birth_approx: input.birth_approx ?? false,
        death_date: input.death_date ?? null,
        bio: input.bio ?? null,
        residence: input.residence ?? null,
        created_at: '2026-11-10T10:00:00Z',
      };
      store.set(created.id, created);
      return jsonResponse(201, created);
    }

    const relation = /^\/persons\/(\d+)\/(parents|partners)(?:\/(\d+))?$/.exec(path);
    if (relation) {
      const id = Number(relation[1]);
      const other = relation[3] === undefined ? NaN : Number(relation[3]);
      if (!store.has(id)) return errorResponse(404, 'person_not_found');
      if (relation[2] === 'parents') {
        if (call.method === 'POST') {
          const parentId = Number(body(call).parent_id);
          if (!store.has(parentId)) return errorResponse(404, 'person_not_found');
          if (parentId === id)
            return errorResponse(400, 'self_parent', 'a person cannot be their own parent');
          if (links.some((l) => l.child_id === id && l.parent_id === parentId)) {
            return errorResponse(409, 'duplicate_parent', 'already a parent');
          }
          if (links.filter((l) => l.child_id === id).length >= 2) {
            return errorResponse(409, 'too_many_parents', 'already has two parents');
          }
          if (isDescendant(id, parentId)) {
            return errorResponse(409, 'would_create_cycle', 'would create a cycle');
          }
          links.push({ parent_id: parentId, child_id: id });
          return jsonResponse(201, { child_id: id, parent_id: parentId });
        }
        if (call.method === 'DELETE') {
          const at = links.findIndex((l) => l.child_id === id && l.parent_id === other);
          if (at < 0) return errorResponse(404, 'parent_link_not_found', 'not linked');
          links.splice(at, 1);
          return new Response(null, { status: 204 });
        }
      } else {
        if (call.method === 'POST') {
          const partnerId = Number(body(call).partner_id);
          if (!store.has(partnerId)) return errorResponse(404, 'person_not_found');
          if (partnerId === id) return errorResponse(400, 'self_partner', 'not themselves');
          if (findPartnership(id, partnerId)) {
            return errorResponse(409, 'duplicate_partnership', 'already partners');
          }
          const kind = text(body(call).kind, 'marriage');
          partnerships.push({ partners: [id, partnerId], kind });
          return jsonResponse(201, { partner_id: partnerId, kind });
        }
        const existing = findPartnership(id, other);
        if (!existing) return errorResponse(404, 'partnership_not_found', 'not partners');
        if (call.method === 'PATCH') {
          existing.kind = text(body(call).kind, existing.kind);
          return jsonResponse(200, { partner_id: other, kind: existing.kind });
        }
        if (call.method === 'DELETE') {
          partnerships.splice(partnerships.indexOf(existing), 1);
          return new Response(null, { status: 204 });
        }
      }
    }

    const one = /^\/persons\/(\d+)$/.exec(path);
    const id = one?.[1] === undefined ? NaN : Number(one[1]);
    const found = store.get(id);
    if (!found) return errorResponse(404, 'person_not_found', 'person not found');
    if (call.method === 'GET') return jsonResponse(200, found);
    if (call.method === 'PATCH') {
      Object.assign(found, body(call));
      return jsonResponse(200, found);
    }
    if (call.method === 'DELETE') {
      store.delete(id);
      return new Response(null, { status: 204 });
    }
    return errorResponse(404, 'not_found');
  };
  return { handler, intercept, store, links, partnerships };
}
