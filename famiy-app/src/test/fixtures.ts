import families from '../../scripts/fixtures/families.json';
import type { ApiGraph } from '../api/types';

interface FixtureFamily {
  persons: {
    key: string;
    given_name: string;
    family_name: string | null;
    birth_year: number | null;
  }[];
  parents: string[][];
  partners: string[][];
}

export const messy: FixtureFamily = families.messy;
export const edgeCases: FixtureFamily = families.edgeCases;

/**
 * Converts a fixture family into the API's `/graph` shape. Ids are 1..n in file order, or in
 * reverse order of creation when `reverse` is set (to prove nothing depends on input order).
 * Years become YYYY-01-01 with `birth_approx` true, like the seed script.
 */
export function apiGraphFrom(
  family: FixtureFamily,
  options: { reverse?: boolean } = {},
): { api: ApiGraph; idOf: Record<string, number> } {
  const idOf: Record<string, number> = {};
  family.persons.forEach((p, index) => {
    idOf[p.key] = index + 1;
  });
  const id = (key: string): number => {
    const value = idOf[key];
    if (value === undefined) throw new Error(`fixture key ${key} is not a person`);
    return value;
  };
  const persons = family.persons.map((p) => ({
    id: id(p.key),
    given_name: p.given_name,
    family_name: p.family_name,
    birth_date: p.birth_year === null ? null : `${String(p.birth_year)}-01-01`,
    birth_approx: p.birth_year !== null,
    death_date: null,
  }));
  const parent_links = family.parents.map(([parent, child]) => ({
    parent_id: id(parent ?? ''),
    child_id: id(child ?? ''),
  }));
  const partnerships = family.partners.map(([a, b, kind]) => ({
    partners: [id(a ?? ''), id(b ?? '')],
    kind: kind ?? 'marriage',
  }));
  if (options.reverse) {
    persons.reverse();
    parent_links.reverse();
    partnerships.reverse();
  }
  return { api: { persons, parent_links, partnerships }, idOf };
}

/** Adds people and links to a graph without touching the original. */
export function emptyGraph(): ApiGraph {
  return { persons: [], parent_links: [], partnerships: [] };
}
