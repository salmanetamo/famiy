import { describe, expect, it } from 'vitest';
import type { PersonDetail } from '../api/types';
import {
  emptyPersonForm,
  formFromPerson,
  toCreateBody,
  toPatchBody,
  validatePersonForm,
  type PersonFormValues,
} from './personForm';

const stored: PersonDetail = {
  id: 7,
  given_name: 'Ada',
  family_name: 'Lovelace',
  birth_date: '1815-12-10',
  birth_approx: false,
  death_date: '1852-11-27',
  bio: 'Mathematician.',
  residence: 'London',
  created_at: '2026-11-01T10:00:00Z',
};
const original = formFromPerson(stored);
const edit = (change: Partial<PersonFormValues>): PersonFormValues => ({ ...original, ...change });

describe('formFromPerson', () => {
  it('turns missing optional fields into empty strings', () => {
    const form = formFromPerson({
      ...stored,
      family_name: null,
      birth_date: null,
      death_date: null,
      bio: null,
      residence: null,
    });
    expect(form).toEqual({ ...emptyPersonForm, givenName: 'Ada' });
  });
});

describe('validatePersonForm', () => {
  it('requires a given name, ignoring spaces', () => {
    expect(validatePersonForm(edit({ givenName: '   ' })).givenName).toBeDefined();
    expect(validatePersonForm(original)).toEqual({});
  });

  it('rejects "approximate" with no birth date', () => {
    const errors = validatePersonForm(edit({ birthDate: '', birthApprox: true }));
    expect(errors.birthDate).toMatch(/approximate/);
  });

  it('rejects a death before the birth, but accepts the same day', () => {
    expect(validatePersonForm(edit({ deathDate: '1800-01-01' })).deathDate).toBeDefined();
    expect(validatePersonForm(edit({ deathDate: '1815-12-10' }))).toEqual({});
  });

  it('does not judge a death date when there is no birth date', () => {
    expect(validatePersonForm(edit({ birthDate: '', deathDate: '1800-01-01' }))).toEqual({});
  });
});

describe('toCreateBody', () => {
  it('trims text and sends blank optional fields as null', () => {
    expect(
      toCreateBody({ ...emptyPersonForm, givenName: '  Nia ', familyName: '  ', bio: '\n' }),
    ).toEqual({
      given_name: 'Nia',
      family_name: null,
      birth_date: null,
      birth_approx: false,
      death_date: null,
      bio: null,
      residence: null,
    });
  });

  it('never sends "approximate" without a birth date', () => {
    expect(
      toCreateBody({ ...emptyPersonForm, givenName: 'Nia', birthApprox: true }).birth_approx,
    ).toBe(false);
  });

  it('keeps line breaks inside the notes', () => {
    expect(toCreateBody({ ...emptyPersonForm, givenName: 'Nia', bio: ' a\nb ' }).bio).toBe('a\nb');
  });
});

describe('toPatchBody', () => {
  it('is empty when nothing changed, even if only spaces differ', () => {
    expect(toPatchBody(original, original)).toEqual({});
    expect(toPatchBody(edit({ givenName: ' Ada ', residence: 'London  ' }), original)).toEqual({});
  });

  it('sends only the fields that changed', () => {
    expect(toPatchBody(edit({ givenName: 'Augusta' }), original)).toEqual({
      given_name: 'Augusta',
    });
    expect(toPatchBody(edit({ bio: 'Wrote the first program.' }), original)).toEqual({
      bio: 'Wrote the first program.',
    });
  });

  it('sends null for a cleared optional field, which is how the API clears it', () => {
    expect(toPatchBody(edit({ familyName: '' }), original)).toEqual({ family_name: null });
    expect(toPatchBody(edit({ residence: '  ' }), original)).toEqual({ residence: null });
    expect(toPatchBody(edit({ deathDate: '' }), original)).toEqual({ death_date: null });
  });

  it('clears the approximate flag together with the birth date it belongs to', () => {
    const approximate = formFromPerson({ ...stored, birth_approx: true });
    expect(toPatchBody({ ...approximate, birthDate: '' }, approximate)).toEqual({
      birth_date: null,
      birth_approx: false,
    });
  });

  it('can turn "approximate" on and off', () => {
    expect(toPatchBody(edit({ birthApprox: true }), original)).toEqual({ birth_approx: true });
  });
});
