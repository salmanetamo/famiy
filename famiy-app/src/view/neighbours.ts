import type { FamilyGraph } from '../domain/family';
import type { Layout, PositionedPerson } from '../layout/types';

export type Direction = 'up' | 'down' | 'left' | 'right';

/**
 * Where the arrow keys move between people: up to a parent, down to a child (both in the usual
 * display order, so "first" is the eldest), left and right to the next person in the same row.
 * Returns null when there is nowhere to go. Pure, so the keyboard behaviour is testable.
 */
export function neighbour(
  layout: Layout,
  family: FamilyGraph,
  fromId: number,
  direction: Direction,
): number | null {
  const positioned = new Map(layout.persons.map((p) => [p.id, p]));
  const from = positioned.get(fromId);
  if (!from) return null;
  const present = (ids: readonly number[] | undefined) =>
    ids?.find((id) => positioned.has(id)) ?? null;

  if (direction === 'up') return present(family.parentsOf.get(fromId));
  if (direction === 'down') return present(family.childrenOf.get(fromId));

  // Rows are exact: every person in a generation shares one y (see the layout tests).
  const row: PositionedPerson[] = layout.persons
    .filter((p) => Math.abs(p.y - from.y) < 1)
    .sort((a, b) => a.x - b.x || a.id - b.id);
  const index = row.findIndex((p) => p.id === fromId);
  const next = row[index + (direction === 'right' ? 1 : -1)];
  return next?.id ?? null;
}
