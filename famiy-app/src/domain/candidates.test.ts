import { describe, expect, it } from 'vitest';
import { apiGraphFrom, messy } from '../test/fixtures';
import { childCandidates, parentCandidates, partnerCandidates } from './candidates';
import { ancestorsOf, buildFamilyGraph, descendantsOf, relativesOf, type Person } from './family';

/*
 * The messy family: Abou+Fati (g1,g2) → Idi (p1), Nana (p2); Idi has partners Rama (s1) and Zara (s2),
 * Nana has Moussa (s3); Ali (c1), Hawa (c2) are Idi+Rama's; Bina (c3) is Idi+Zara's; Sala (c4) is
 * Nana+Moussa's; Umar (d1) is Ali+Sala's. Everyone with a parent has two.
 */
const { api, idOf } = apiGraphFrom(messy);
const family = buildFamilyGraph(api);
const id = (key: string): number => idOf[key] ?? -1;
const names = (people: readonly Person[]) => people.map((p) => p.givenName).sort();

describe('descendantsOf / ancestorsOf', () => {
  it('follow every generation, not only the next one', () => {
    expect(
      names([...descendantsOf(family, id('g1'))].flatMap((x) => family.personById.get(x) ?? [])),
    ).toEqual(['Ali', 'Bina', 'Hawa', 'Idi', 'Nana', 'Sala', 'Umar']);
    expect(
      names([...ancestorsOf(family, id('d1'))].flatMap((x) => family.personById.get(x) ?? [])),
    ).toEqual(['Abou', 'Ali', 'Fati', 'Idi', 'Moussa', 'Nana', 'Rama', 'Sala']);
  });

  it('never include the person, and are empty for someone with no descendants', () => {
    expect(descendantsOf(family, id('g1')).has(id('g1'))).toBe(false);
    expect(descendantsOf(family, id('d1')).size).toBe(0);
    expect(ancestorsOf(family, id('g1')).size).toBe(0);
  });
});

describe('parentCandidates', () => {
  it('is empty when the person already has two parents', () => {
    expect(parentCandidates(family, id('c2'))).toEqual([]);
    expect(parentCandidates(family, id('d1'))).toEqual([]);
  });

  it('leaves out the person and everyone below them, so no one becomes their own ancestor', () => {
    expect(names(parentCandidates(family, id('g1')))).toEqual(['Fati', 'Moussa', 'Rama', 'Zara']);
    // Moussa's descendants are Sala and Umar
    expect(names(parentCandidates(family, id('s3')))).not.toContain('Umar');
    expect(names(parentCandidates(family, id('s3')))).not.toContain('Sala');
    expect(names(parentCandidates(family, id('s3')))).toContain('Abou');
  });

  it('leaves out a parent they already have', () => {
    const oneParent = buildFamilyGraph({
      persons: [
        {
          id: 1,
          given_name: 'A',
          family_name: null,
          birth_date: null,
          birth_approx: false,
          death_date: null,
        },
        {
          id: 2,
          given_name: 'B',
          family_name: null,
          birth_date: null,
          birth_approx: false,
          death_date: null,
        },
        {
          id: 3,
          given_name: 'C',
          family_name: null,
          birth_date: null,
          birth_approx: false,
          death_date: null,
        },
      ],
      parent_links: [{ parent_id: 1, child_id: 3 }],
      partnerships: [],
    });
    expect(names(parentCandidates(oneParent, 3))).toEqual(['B']);
  });
});

describe('childCandidates', () => {
  it('offers only people who have a free parent slot', () => {
    // every descendant in the family already has two parents
    expect(names(childCandidates(family, id('g1')))).toEqual(['Fati', 'Moussa', 'Rama', 'Zara']);
  });

  it('leaves out ancestors, the person, and their existing children', () => {
    expect(names(childCandidates(family, id('s1')))).toEqual(['Abou', 'Fati', 'Moussa', 'Zara']);
    // Umar's ancestors (Ali, Sala, Idi, Abou, ...) are never offered as his children
    const forUmar = names(childCandidates(family, id('d1')));
    for (const ancestor of ['Ali', 'Sala', 'Idi', 'Rama', 'Abou', 'Fati']) {
      expect(forUmar).not.toContain(ancestor);
    }
  });

  it('does not offer an ancestor as a child (that would be a cycle)', () => {
    const solo = buildFamilyGraph({
      persons: [
        {
          id: 1,
          given_name: 'A',
          family_name: null,
          birth_date: null,
          birth_approx: false,
          death_date: null,
        },
        {
          id: 2,
          given_name: 'B',
          family_name: null,
          birth_date: null,
          birth_approx: false,
          death_date: null,
        },
        {
          id: 3,
          given_name: 'C',
          family_name: null,
          birth_date: null,
          birth_approx: false,
          death_date: null,
        },
      ],
      parent_links: [
        { parent_id: 1, child_id: 2 },
        { parent_id: 2, child_id: 3 },
      ],
      partnerships: [],
    });
    // for C, both A and B are ancestors; for B, A is an ancestor and C is already a child
    expect(names(childCandidates(solo, 3))).toEqual([]);
    expect(names(childCandidates(solo, 2))).toEqual([]);
    // for A, C is allowed: a grandchild may also be a direct child, and that makes no cycle
    expect(names(childCandidates(solo, 1))).toEqual(['C']);
  });

  it('counts a person with exactly one parent as having a free slot', () => {
    const oneParent = buildFamilyGraph({
      persons: [
        {
          id: 1,
          given_name: 'A',
          family_name: null,
          birth_date: null,
          birth_approx: false,
          death_date: null,
        },
        {
          id: 2,
          given_name: 'B',
          family_name: null,
          birth_date: null,
          birth_approx: false,
          death_date: null,
        },
        {
          id: 3,
          given_name: 'C',
          family_name: null,
          birth_date: null,
          birth_approx: false,
          death_date: null,
        },
      ],
      parent_links: [{ parent_id: 1, child_id: 3 }],
      partnerships: [],
    });
    expect(names(childCandidates(oneParent, 2))).toEqual(['A', 'C']);
  });
});

describe('partnerCandidates', () => {
  it('leaves out the person and everyone they already have a recorded partnership with', () => {
    const offered = names(partnerCandidates(family, id('p1')));
    expect(offered).not.toContain('Idi');
    expect(offered).not.toContain('Rama');
    expect(offered).not.toContain('Zara');
    expect(offered).toHaveLength(family.persons.length - 3);
  });

  it('allows blood relatives, which genealogies contain', () => {
    expect(names(partnerCandidates(family, id('p1')))).toContain('Nana'); // a sibling
    expect(names(partnerCandidates(family, id('p1')))).toContain('Abou'); // a parent
  });

  it('offers two people who only share a child, since no partnership is recorded', () => {
    const shared = buildFamilyGraph({
      persons: [
        {
          id: 1,
          given_name: 'A',
          family_name: null,
          birth_date: null,
          birth_approx: false,
          death_date: null,
        },
        {
          id: 2,
          given_name: 'B',
          family_name: null,
          birth_date: null,
          birth_approx: false,
          death_date: null,
        },
        {
          id: 3,
          given_name: 'C',
          family_name: null,
          birth_date: null,
          birth_approx: false,
          death_date: null,
        },
      ],
      parent_links: [
        { parent_id: 1, child_id: 3 },
        { parent_id: 2, child_id: 3 },
      ],
      partnerships: [],
    });
    expect(names(partnerCandidates(shared, 1))).toEqual(['B', 'C']);
    expect(relativesOf(shared, 1).partnerKinds.get(2)).toBeNull();
  });
});

describe('relativesOf partner kinds', () => {
  it('reports the recorded kind of each partnership', () => {
    const kinds = relativesOf(family, id('p1')).partnerKinds;
    expect(kinds.get(id('s1'))).toBe('marriage');
    expect(kinds.get(id('s2'))).toBe('marriage');
  });
});
