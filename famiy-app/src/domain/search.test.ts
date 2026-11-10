import { describe, expect, it } from 'vitest';
import type { Person } from './family';
import { buildSearchIndex, searchPeople } from './search';

const p = (id: number, givenName: string, familyName: string | null): Person => ({
  id,
  givenName,
  familyName,
  birthDate: null,
  birthApprox: false,
  deathDate: null,
});

const people = [
  p(1, 'Zoë', 'Martin'),
  p(2, 'Ada', 'Lovelace'),
  p(3, 'Ali', 'Diallo'),
  p(4, 'Alice', null),
  p(5, 'Ali', 'Abate'),
  p(6, 'Ali', 'Abate'),
  p(7, 'Zoe', 'Plain'),
  p(8, 'Éloïse', 'Roy'),
];
const index = buildSearchIndex(people);
const ids = (query: string, limit = 50) =>
  searchPeople(index, query, limit).people.map((person) => person.id);

describe('searchPeople', () => {
  it('matches case-insensitively, anywhere in the name', () => {
    expect(ids('LOVE')).toEqual([2]);
    expect(ids('ovela')).toEqual([2]);
  });

  it('requires every word, in any order', () => {
    expect(ids('diallo ali')).toEqual([3]);
    expect(ids('ali lovelace')).toEqual([]);
  });

  it('ignores extra spaces around and between words', () => {
    expect(ids('   ali    diallo ')).toEqual([3]);
  });

  it('ignores accents in both directions', () => {
    expect(ids('zoe')).toEqual([1, 7]);
    expect(ids('ZOË')).toEqual([1, 7]); // typed with the accent, stored without
    expect(ids('eloise')).toEqual([8]); // stored with two accents inside the word
    expect(ids('ÉLOÏSE')).toEqual([8]);
    expect(ids('zoë mart')).toEqual([1]);
  });

  it('finds nobody for a blank query', () => {
    expect(searchPeople(index, '', 8)).toEqual({ people: [], total: 0 });
    expect(searchPeople(index, '   ', 8)).toEqual({ people: [], total: 0 });
  });

  it('orders like the server: family name (none last), given name, then id', () => {
    // Abate, Abate (by id), Diallo, then Alice who has no family name
    expect(ids('ali')).toEqual([5, 6, 3, 4]);
  });

  it('reports the full count while listing only up to the limit', () => {
    const result = searchPeople(index, 'ali', 2);
    expect(result.people.map((person) => person.id)).toEqual([5, 6]);
    expect(result.total).toBe(4);
  });

  it('does not treat query characters as patterns', () => {
    expect(ids('%')).toEqual([]);
    expect(ids('a.')).toEqual([]);
    expect(ids('(')).toEqual([]);
  });
});
