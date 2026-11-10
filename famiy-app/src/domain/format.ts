import type { Person } from './family';

export function displayName(person: Person): string {
  return person.familyName ? `${person.givenName} ${person.familyName}` : person.givenName;
}

/** The year of an ISO date (`YYYY-MM-DD`). Years are always four digits in the API (1000 and later). */
const yearOf = (isoDate: string): string => isoDate.slice(0, 4);

/**
 * A short life span for a card or a label: "1930–2001", "b. 1930", "d. 2001", or with an approximate
 * birth year "c. 1930" and "c. 1930–2001"; "" when no dates are known.
 */
export function lifespan(person: Person): string {
  const born = person.birthDate === null ? null : yearOf(person.birthDate);
  const died = person.deathDate === null ? null : yearOf(person.deathDate);
  const bornText = born === null ? null : `${person.birthApprox ? 'c. ' : ''}${born}`;
  if (bornText !== null && died !== null) return `${bornText}–${died}`;
  if (bornText !== null) return person.birthApprox ? bornText : `b. ${bornText}`;
  if (died !== null) return `d. ${died}`;
  return '';
}

/**
 * What a screen reader hears for a person in the graph. Whether they are selected is not part of the
 * name: the card is a toggle button, and `aria-pressed` already announces that state.
 */
export function personLabel(person: Person): string {
  const span = lifespan(person);
  return span ? `${displayName(person)}, ${span}` : displayName(person);
}

const segmenter = new Intl.Segmenter();

/**
 * Shortens `text` to at most `max` visible characters (grapheme clusters, so an accented letter or
 * a family emoji is never cut in half), ending in an ellipsis when it was cut.
 */
export function truncate(text: string, max: number): string {
  const graphemes = Array.from(segmenter.segment(text), (part) => part.segment);
  if (graphemes.length <= max) return text;
  return `${graphemes
    .slice(0, Math.max(0, max - 1))
    .join('')
    .trimEnd()}…`;
}
