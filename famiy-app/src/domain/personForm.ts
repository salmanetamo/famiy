import type { CreatePersonBody, PatchPersonBody, PersonDetail } from '../api/types';

/** What the person form holds: plain strings as typed ('' = empty), so inputs stay controlled. */
export interface PersonFormValues {
  givenName: string;
  familyName: string;
  /** ISO date `YYYY-MM-DD` from a date input, or ''. */
  birthDate: string;
  birthApprox: boolean;
  deathDate: string;
  residence: string;
  bio: string;
}

export const emptyPersonForm: PersonFormValues = {
  givenName: '',
  familyName: '',
  birthDate: '',
  birthApprox: false,
  deathDate: '',
  residence: '',
  bio: '',
};

export function formFromPerson(person: PersonDetail): PersonFormValues {
  return {
    givenName: person.given_name,
    familyName: person.family_name ?? '',
    birthDate: person.birth_date ?? '',
    birthApprox: person.birth_approx,
    deathDate: person.death_date ?? '',
    residence: person.residence ?? '',
    bio: person.bio ?? '',
  };
}

export type PersonFormErrors = Partial<Record<'givenName' | 'birthDate' | 'deathDate', string>>;

/**
 * Only the checks that save a round trip. The server decides everything else (date range, lengths,
 * the account's person limit) and its message is shown if it refuses.
 */
export function validatePersonForm(values: PersonFormValues): PersonFormErrors {
  const errors: PersonFormErrors = {};
  if (values.givenName.trim() === '') errors.givenName = 'Enter a given name.';
  if (values.birthApprox && values.birthDate === '') {
    errors.birthDate = 'Enter a birth date, or untick "approximate".';
  }
  // ISO dates compare correctly as text
  if (values.birthDate !== '' && values.deathDate !== '' && values.deathDate < values.birthDate) {
    errors.deathDate = 'The death date cannot be before the birth date.';
  }
  return errors;
}

/** The values as the API stores them: trimmed, with blank optional fields as null. */
interface Normalised {
  given_name: string;
  family_name: string | null;
  birth_date: string | null;
  birth_approx: boolean;
  death_date: string | null;
  bio: string | null;
  residence: string | null;
}

const orNull = (text: string): string | null => (text.trim() === '' ? null : text.trim());

function normalise(values: PersonFormValues): Normalised {
  const birth = values.birthDate === '' ? null : values.birthDate;
  return {
    given_name: values.givenName.trim(),
    family_name: orNull(values.familyName),
    birth_date: birth,
    // an approximate flag without a birth date is invalid, so clearing the date clears it too
    birth_approx: birth !== null && values.birthApprox,
    death_date: values.deathDate === '' ? null : values.deathDate,
    bio: orNull(values.bio),
    residence: orNull(values.residence),
  };
}

export function toCreateBody(values: PersonFormValues): CreatePersonBody {
  return normalise(values);
}

/** Only what changed. An empty object means there is nothing to save. */
export function toPatchBody(values: PersonFormValues, original: PersonFormValues): PatchPersonBody {
  const next = normalise(values);
  const before = normalise(original);
  const patch: PatchPersonBody = {};
  if (next.given_name !== before.given_name) patch.given_name = next.given_name;
  if (next.family_name !== before.family_name) patch.family_name = next.family_name;
  if (next.birth_date !== before.birth_date) patch.birth_date = next.birth_date;
  if (next.birth_approx !== before.birth_approx) patch.birth_approx = next.birth_approx;
  if (next.death_date !== before.death_date) patch.death_date = next.death_date;
  if (next.bio !== before.bio) patch.bio = next.bio;
  if (next.residence !== before.residence) patch.residence = next.residence;
  return patch;
}
