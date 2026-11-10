import { describe, expect, it } from 'vitest';
import type { ApiGraph } from '../api/types';
import { apiGraphFrom, edgeCases, emptyGraph, messy } from '../test/fixtures';
import { buildFamilyGraph, comparePeople, type Person } from './family';

const person = (id: number, birth: string | null): ApiGraph['persons'][number] => ({
  id,
  given_name: `P${String(id)}`,
  family_name: null,
  birth_date: birth,
  birth_approx: false,
  death_date: null,
});

describe('comparePeople', () => {
  const p = (id: number, birthDate: string | null): Person => ({
    id,
    givenName: 'x',
    familyName: null,
    birthDate,
    birthApprox: false,
    deathDate: null,
  });

  it('orders by birth date ascending', () => {
    expect(comparePeople(p(2, '1950-01-01'), p(1, '1960-01-01'))).toBeLessThan(0);
  });

  it('puts people with no birth date after people with one, whatever their ids', () => {
    expect(comparePeople(p(1, null), p(2, '1990-01-01'))).toBeGreaterThan(0);
    expect(comparePeople(p(2, '1990-01-01'), p(1, null))).toBeLessThan(0);
  });

  it('breaks ties (same date, or both unknown) by id', () => {
    expect(comparePeople(p(1, '1950-01-01'), p(2, '1950-01-01'))).toBeLessThan(0);
    expect(comparePeople(p(3, null), p(2, null))).toBeGreaterThan(0);
  });
});

describe('buildFamilyGraph', () => {
  it('handles an empty account', () => {
    const family = buildFamilyGraph(emptyGraph());
    expect(family.persons).toEqual([]);
    expect(family.unions).toEqual([]);
  });

  it('builds the messy family: five unions, remarried person in two, half-siblings under different unions', () => {
    const { api, idOf } = apiGraphFrom(messy);
    const family = buildFamilyGraph(api);
    const id = (key: string) => idOf[key] ?? -1;
    expect(family.unions).toHaveLength(5);
    const unionsOfIdi = family.unions.filter((u) => u.partners.includes(id('p1')));
    expect(unionsOfIdi).toHaveLength(2);
    const withRama = unionsOfIdi.find((u) => u.partners.includes(id('s1')));
    const withZara = unionsOfIdi.find((u) => u.partners.includes(id('s2')));
    expect(withRama?.children).toEqual([id('c1'), id('c2')]);
    expect(withZara?.children).toEqual([id('c3')]);
    expect(family.parentsOf.get(id('d1'))).toEqual([id('c1'), id('c4')]);
    expect(family.childrenOf.get(id('p1'))).toEqual([id('c1'), id('c2'), id('c3')]);
  });

  it('makes a union from shared children even when no partnership is recorded', () => {
    const { api, idOf } = apiGraphFrom(edgeCases);
    const family = buildFamilyGraph(api);
    const noor = idOf.noor ?? -1;
    const omar = idOf.omar ?? -1;
    const lina = idOf.lina ?? -1;
    const union = family.unions.find((u) => u.partners.includes(noor) && u.partners.includes(omar));
    expect(union).toBeDefined();
    expect(union?.kind).toBeNull();
    expect(union?.children).toEqual([lina]);
  });

  it('keeps a childless partnership as a union with the recorded kind', () => {
    const { api, idOf } = apiGraphFrom(edgeCases);
    const family = buildFamilyGraph(api);
    const union = family.unions.find((u) => u.partners.includes(idOf.pia ?? -1));
    expect(union?.kind).toBe('partnership');
    expect(union?.children).toEqual([]);
  });

  it('attaches a child with one known parent to that parent, not to a union', () => {
    const { api, idOf } = apiGraphFrom(edgeCases);
    const family = buildFamilyGraph(api);
    const rosa = idOf.rosa ?? -1;
    expect(family.singleParentChildren.get(rosa)).toEqual([idOf.sami, idOf.tara]);
    expect(family.unions.some((u) => u.partners.includes(rosa))).toBe(false);
  });

  it('orders a sibling with no birth date after the one with a date', () => {
    const { api, idOf } = apiGraphFrom(edgeCases);
    const family = buildFamilyGraph(api);
    const rosa = idOf.rosa ?? -1;
    // tara has no birth date and a higher id than sami, but even with a lower id she sorts last
    expect(family.childrenOf.get(rosa)).toEqual([idOf.sami, idOf.tara]);
    const flipped = buildFamilyGraph({
      ...api,
      persons: api.persons.map((p) => (p.id === idOf.tara ? { ...p, id: 0 } : p)),
      parent_links: api.parent_links.map((l) =>
        l.child_id === idOf.tara ? { ...l, child_id: 0 } : l,
      ),
    });
    expect(flipped.childrenOf.get(rosa)).toEqual([idOf.sami, 0]);
  });

  it('does not depend on the order of the input', () => {
    const forward = buildFamilyGraph(apiGraphFrom(messy).api);
    const backward = buildFamilyGraph(apiGraphFrom(messy, { reverse: true }).api);
    expect(backward.persons.map((p) => p.id)).toEqual(forward.persons.map((p) => p.id));
    expect(backward.unions).toEqual(forward.unions);
  });

  it('skips links and partnerships that name unknown people instead of crashing', () => {
    const family = buildFamilyGraph({
      persons: [person(1, '1950-01-01'), person(2, '1980-01-01')],
      parent_links: [
        { parent_id: 1, child_id: 2 },
        { parent_id: 99, child_id: 2 },
      ],
      partnerships: [
        { partners: [1, 99], kind: 'marriage' },
        { partners: [2, 2], kind: 'marriage' },
      ],
    });
    expect(family.parentsOf.get(2)).toEqual([1]);
    expect(family.unions).toEqual([]);
    expect(family.personById.size).toBe(2);
  });

  it('ignores a duplicated parent link', () => {
    const family = buildFamilyGraph({
      persons: [person(1, null), person(2, null)],
      parent_links: [
        { parent_id: 1, child_id: 2 },
        { parent_id: 1, child_id: 2 },
      ],
      partnerships: [],
    });
    expect(family.parentsOf.get(2)).toEqual([1]);
    expect(family.childrenOf.get(1)).toEqual([2]);
  });
});
