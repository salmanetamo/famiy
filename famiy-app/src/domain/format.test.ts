import { describe, expect, it } from 'vitest';
import type { Person } from './family';
import { displayName, lifespan, personLabel, truncate } from './format';

const base: Person = {
  id: 1,
  givenName: 'Ada',
  familyName: 'Lovelace',
  birthDate: null,
  birthApprox: false,
  deathDate: null,
};
const person = (overrides: Partial<Person>): Person => ({ ...base, ...overrides });

describe('displayName', () => {
  it('joins given and family name, or shows just the given name', () => {
    expect(displayName(base)).toBe('Ada Lovelace');
    expect(displayName(person({ familyName: null }))).toBe('Ada');
  });
});

describe('lifespan', () => {
  it.each([
    [{}, ''],
    [{ birthDate: '1930-05-01' }, 'b. 1930'],
    [{ birthDate: '1930-05-01', birthApprox: true }, 'c. 1930'],
    [{ deathDate: '2001-02-03' }, 'd. 2001'],
    [{ birthDate: '1930-05-01', deathDate: '2001-02-03' }, '1930–2001'],
    [{ birthDate: '1930-01-01', birthApprox: true, deathDate: '2001-02-03' }, 'c. 1930–2001'],
  ] as const)('%j -> %j', (overrides, expected) => {
    expect(lifespan(person(overrides))).toBe(expected);
  });
});

describe('personLabel', () => {
  it('is the name plus the life span when known, and never says "selected" (aria-pressed does)', () => {
    expect(personLabel(base)).toBe('Ada Lovelace');
    expect(personLabel(person({ birthDate: '1815-12-10', deathDate: '1852-11-27' }))).toBe(
      'Ada Lovelace, 1815–1852',
    );
  });
});

describe('truncate', () => {
  it('leaves short text alone and cuts long text with an ellipsis within the limit', () => {
    expect(truncate('Ada', 10)).toBe('Ada');
    expect(truncate('Ada Lovelace-King', 10)).toBe('Ada Lovel…');
    expect(truncate('x'.repeat(50), 12)).toHaveLength(12); // 11 letters and the one-unit ellipsis
  });

  it('cuts between visible characters, never inside an emoji sequence or a combining accent', () => {
    const family = '👩‍👩‍👧'; // one visible character made of several code points
    expect(truncate(family.repeat(5), 3)).toBe(`${family}${family}…`);
    const accented = 'é'; // e + combining acute accent
    expect(truncate(accented.repeat(5), 3)).toBe(`${accented}${accented}…`);
  });

  it('does not leave a space before the ellipsis', () => {
    expect(truncate('Ada Lovelace', 5)).toBe('Ada…');
  });
});
