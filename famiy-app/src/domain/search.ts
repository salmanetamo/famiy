import type { Person } from './family';

/** Lower-case and strip accents, so "Zoe" finds "Zoë" (the server's own search does not do this). */
const fold = (text: string): string => text.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();

interface Entry {
  person: Person;
  /** Folded "given family", what every word of a query must appear in. */
  text: string;
  /** Folded sort keys, mirroring the server: family name (none last), given name, id. */
  family: string | null;
  given: string;
}

export type SearchIndex = readonly Entry[];

export interface SearchResult {
  /** At most `limit` matches, in the server's order. */
  people: readonly Person[];
  /** How many people matched in all. */
  total: number;
}

/** Built once per graph, so typing does not re-fold every name on every keystroke. */
export function buildSearchIndex(persons: readonly Person[]): SearchIndex {
  return persons.map((person) => {
    const given = fold(person.givenName);
    const family = person.familyName === null ? null : fold(person.familyName);
    return { person, text: family === null ? given : `${given} ${family}`, family, given };
  });
}

function compareEntries(a: Entry, b: Entry): number {
  if (a.family !== b.family) {
    if (a.family === null) return 1;
    if (b.family === null) return -1;
    if (a.family !== b.family) return a.family < b.family ? -1 : 1;
  }
  if (a.given !== b.given) return a.given < b.given ? -1 : 1;
  return a.person.id - b.person.id;
}

/**
 * People whose name contains every word of `query`, ignoring case and accents. The same rule as
 * `GET /persons?q=` (and the same order), so the box behaves the way the API documents.
 */
export function searchPeople(index: SearchIndex, query: string, limit: number): SearchResult {
  const words = fold(query).split(/\s+/).filter(Boolean);
  if (words.length === 0) return { people: [], total: 0 };
  const matches = index.filter((entry) => words.every((word) => entry.text.includes(word)));
  matches.sort(compareEntries);
  return { people: matches.slice(0, limit).map((entry) => entry.person), total: matches.length };
}
