import type { ApiGraph, PersonSummary } from '../api/types';

export interface Person {
  id: number;
  givenName: string;
  familyName: string | null;
  /** ISO date `YYYY-MM-DD`, or null when unknown. */
  birthDate: string | null;
  birthApprox: boolean;
  deathDate: string | null;
}

/**
 * A couple, as the layout needs it. It exists when two people are recorded as partners OR when
 * they share a child (a child can have two known parents with no recorded partnership), and a
 * partnership with no children still gets one.
 */
export interface Union {
  /** Stable id from the two person ids, smaller first, so a pair is never listed twice. */
  id: string;
  partners: readonly [number, number];
  /** The recorded partnership kind, or null when the union only exists because of shared children. */
  kind: string | null;
  /** Children of exactly these two parents, in display order. */
  children: readonly number[];
}

export interface FamilyGraph {
  /** All people in display order (birth date ascending, unknown dates last, then id). */
  persons: readonly Person[];
  personById: ReadonlyMap<number, Person>;
  /** Known parents of each person (0, 1 or 2), in display order. Absent key = no known parents. */
  parentsOf: ReadonlyMap<number, readonly number[]>;
  /** Children of each person across all unions and single-parent links, in display order. */
  childrenOf: ReadonlyMap<number, readonly number[]>;
  unions: readonly Union[];
  /** Children with exactly one known parent, grouped by that parent. */
  singleParentChildren: ReadonlyMap<number, readonly number[]>;
}

/**
 * The one ordering used everywhere (layers, siblings, neighbours): birth date ascending, people
 * with no birth date after those with one, ties broken by id so the order is always the same.
 * ISO dates compare correctly as strings.
 */
export function comparePeople(a: Person, b: Person): number {
  if (a.birthDate !== null && b.birthDate !== null) {
    if (a.birthDate !== b.birthDate) return a.birthDate < b.birthDate ? -1 : 1;
  } else if (a.birthDate !== null) {
    return -1;
  } else if (b.birthDate !== null) {
    return 1;
  }
  return a.id - b.id;
}

export interface Relatives {
  parents: readonly Person[];
  partners: readonly Person[];
  children: readonly Person[];
}

/** Direct relatives of one person, each list in display order. */
export function relativesOf(family: FamilyGraph, id: number): Relatives {
  const pick = (ids: readonly number[] | undefined): Person[] =>
    (ids ?? []).flatMap((other) => family.personById.get(other) ?? []);
  const partnerIds = family.unions.flatMap((union) =>
    union.partners[0] === id
      ? [union.partners[1]]
      : union.partners[1] === id
        ? [union.partners[0]]
        : [],
  );
  // a union can exist only because of shared children; those two are still partners to the reader
  return {
    parents: pick(family.parentsOf.get(id)),
    partners: pick(partnerIds).sort(comparePeople),
    children: pick(family.childrenOf.get(id)),
  };
}

function toPerson(p: PersonSummary): Person {
  return {
    id: p.id,
    givenName: p.given_name,
    familyName: p.family_name,
    birthDate: p.birth_date,
    birthApprox: p.birth_approx,
    deathDate: p.death_date,
  };
}

const unionId = (a: number, b: number): string =>
  `${String(Math.min(a, b))}-${String(Math.max(a, b))}`;

function pushTo(map: Map<number, number[]>, key: number, value: number): void {
  const list = map.get(key);
  if (list) list.push(value);
  else map.set(key, [value]);
}

/**
 * Normalises the API's graph into the shape layout and rendering use. Pure. Links and partnerships
 * that mention an unknown person are skipped rather than crashing.
 */
export function buildFamilyGraph(api: ApiGraph): FamilyGraph {
  const persons = api.persons.map(toPerson).sort(comparePeople);
  const personById = new Map(persons.map((p) => [p.id, p]));
  const order = new Map(persons.map((p, index) => [p.id, index]));
  const byOrder = (a: number, b: number) => (order.get(a) ?? 0) - (order.get(b) ?? 0);

  const parents = new Map<number, number[]>();
  const children = new Map<number, number[]>();
  const seenLinks = new Set<string>();
  for (const link of api.parent_links) {
    const known = personById.has(link.parent_id) && personById.has(link.child_id);
    if (!known) {
      continue; // cannot happen with the real API (foreign keys); skipped rather than crashing
    }
    const key = `${String(link.parent_id)}>${String(link.child_id)}`;
    if (seenLinks.has(key)) continue;
    seenLinks.add(key);
    pushTo(parents, link.child_id, link.parent_id);
    pushTo(children, link.parent_id, link.child_id);
  }
  for (const list of parents.values()) list.sort(byOrder);
  for (const list of children.values()) list.sort(byOrder);

  const unions = new Map<
    string,
    { partners: [number, number]; kind: string | null; children: number[] }
  >();
  const ensureUnion = (a: number, b: number, kind: string | null) => {
    const id = unionId(a, b);
    let union = unions.get(id);
    if (!union) {
      union = { partners: [Math.min(a, b), Math.max(a, b)], kind, children: [] };
      unions.set(id, union);
    } else if (kind !== null) {
      union.kind = kind;
    }
    return union;
  };

  for (const partnership of api.partnerships) {
    const [a, b] = partnership.partners;
    if (a === undefined || b === undefined || a === b || !personById.has(a) || !personById.has(b)) {
      continue; // same: skipped rather than crashing
    }
    ensureUnion(a, b, partnership.kind);
  }

  const singleParentChildren = new Map<number, number[]>();
  for (const [child, known] of parents) {
    // The database allows at most two parents, so a third would never arrive; it would be ignored.
    const [first, second] = known;
    if (first !== undefined && second !== undefined) {
      ensureUnion(first, second, null).children.push(child);
    } else if (first !== undefined) {
      pushTo(singleParentChildren, first, child);
    }
  }

  const unionList: Union[] = [...unions.entries()]
    .map(([id, u]) => ({
      id,
      partners: u.partners,
      kind: u.kind,
      children: u.children.sort(byOrder),
    }))
    .sort((a, b) => byOrder(a.partners[0], b.partners[0]) || byOrder(a.partners[1], b.partners[1]));

  return {
    persons,
    personById,
    parentsOf: parents,
    childrenOf: children,
    unions: unionList,
    singleParentChildren,
  };
}
