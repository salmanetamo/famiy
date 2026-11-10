import { ancestorsOf, descendantsOf, type FamilyGraph, type Person } from './family';

/** The most parents a person can have (the API refuses a third). */
export const MAX_PARENTS = 2;

/**
 * Who can be chosen for each kind of link. These rules only keep the picker from offering choices the
 * server would refuse (itself, a repeat, a second-too-many parent, a cycle). The server stays the
 * authority and its refusal is shown if the two ever disagree.
 */

/** People who could become a parent of `childId`: not themselves, not already a parent, not a descendant. */
export function parentCandidates(family: FamilyGraph, childId: number): readonly Person[] {
  if ((family.parentsOf.get(childId)?.length ?? 0) >= MAX_PARENTS) return [];
  const excluded = new Set([
    childId,
    ...(family.parentsOf.get(childId) ?? []),
    ...descendantsOf(family, childId),
  ]);
  return family.persons.filter((person) => !excluded.has(person.id));
}

/**
 * People who could become a child of `parentId`: not themselves, not already a child, not an ancestor,
 * and not someone who already has two parents.
 */
export function childCandidates(family: FamilyGraph, parentId: number): readonly Person[] {
  const excluded = new Set([
    parentId,
    ...(family.childrenOf.get(parentId) ?? []),
    ...ancestorsOf(family, parentId),
  ]);
  return family.persons.filter(
    (person) =>
      !excluded.has(person.id) && (family.parentsOf.get(person.id)?.length ?? 0) < MAX_PARENTS,
  );
}

/**
 * People who could be recorded as a partner of `id`: anyone but themselves and people they already
 * have a recorded partnership with. (Blood relatives are allowed; genealogies contain such cases.)
 * Two people who only share a child have no recorded partnership, so they can be offered.
 */
export function partnerCandidates(family: FamilyGraph, id: number): readonly Person[] {
  const recorded = new Set<number>();
  for (const union of family.unions) {
    if (union.kind === null) continue;
    if (union.partners[0] === id) recorded.add(union.partners[1]);
    else if (union.partners[1] === id) recorded.add(union.partners[0]);
  }
  return family.persons.filter((person) => person.id !== id && !recorded.has(person.id));
}
