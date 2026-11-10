// @vitest-environment node
import { beforeAll, describe, expect, it } from 'vitest';
import { buildFamilyGraph, type FamilyGraph } from '../domain/family';
import { layoutFamily } from '../layout/layoutFamily';
import type { Layout } from '../layout/types';
import { apiGraphFrom, messy } from '../test/fixtures';
import { createInlineEngine } from '../test/inlineEngine';
import { neighbour } from './neighbours';

let family: FamilyGraph;
let layout: Layout;
let idOf: Record<string, number>;
const id = (key: string) => idOf[key] ?? -1;

beforeAll(async () => {
  const built = apiGraphFrom(messy);
  idOf = built.idOf;
  family = buildFamilyGraph(built.api);
  layout = await layoutFamily(family, createInlineEngine());
});

describe('neighbour', () => {
  it('goes up to the first (eldest) parent and down to the first (eldest) child', () => {
    expect(neighbour(layout, family, id('d1'), 'up')).toBe(id('c1')); // Ali, born 1982, before Sala 1984
    expect(neighbour(layout, family, id('p1'), 'down')).toBe(id('c1')); // Ali, eldest of Idi's children
  });

  it('has nowhere to go up from a founder or down from a childless person', () => {
    expect(neighbour(layout, family, id('g1'), 'up')).toBeNull();
    expect(neighbour(layout, family, id('d1'), 'down')).toBeNull();
  });

  it('moves left and right along the generation, in left-to-right order, and stops at the ends', () => {
    const row = layout.persons
      .filter((p) => p.y === (layout.persons.find((q) => q.id === id('c1'))?.y ?? -1))
      .sort((a, b) => a.x - b.x);
    expect(row.length).toBeGreaterThan(2);
    const [first, second] = row;
    const last = row[row.length - 1];
    expect(neighbour(layout, family, first?.id ?? -1, 'left')).toBeNull();
    expect(neighbour(layout, family, first?.id ?? -1, 'right')).toBe(second?.id);
    expect(neighbour(layout, family, second?.id ?? -1, 'left')).toBe(first?.id);
    expect(neighbour(layout, family, last?.id ?? -1, 'right')).toBeNull();
  });

  it('visits every person of a generation by pressing right repeatedly', () => {
    const rowY = layout.persons.find((p) => p.id === id('c1'))?.y;
    const row = layout.persons.filter((p) => p.y === rowY);
    let current = row.sort((a, b) => a.x - b.x)[0]?.id ?? -1;
    const seen = [current];
    for (let step = 0; step < 20; step++) {
      const next = neighbour(layout, family, current, 'right');
      if (next === null) break;
      seen.push(next);
      current = next;
    }
    expect(new Set(seen).size).toBe(row.length);
  });

  it('returns null for an unknown person', () => {
    expect(neighbour(layout, family, 9999, 'up')).toBeNull();
  });
});
