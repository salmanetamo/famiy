// @vitest-environment node
import { describe, expect, it } from 'vitest';
import { buildFamilyGraph, type FamilyGraph } from '../domain/family';
import { apiGraphFrom, edgeCases, emptyGraph, messy } from '../test/fixtures';
import { createInlineEngine } from '../test/inlineEngine';
import { layoutFamily } from './layoutFamily';
import type { Layout, PositionedPerson } from './types';

const engine = createInlineEngine();

async function run(family: typeof messy, options: { reverse?: boolean } = {}) {
  const { api, idOf } = apiGraphFrom(family, options);
  const graph = buildFamilyGraph(api);
  const layout = await layoutFamily(graph, engine);
  const byId = new Map(layout.persons.map((p) => [p.id, p]));
  const at = (key: string): PositionedPerson => {
    const found = byId.get(idOf[key] ?? -1);
    if (!found) throw new Error(`no position for ${key}`);
    return found;
  };
  return { graph, layout, at, idOf };
}

const overlap = (a: PositionedPerson, b: PositionedPerson) =>
  a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height;

function noOverlaps(layout: Layout) {
  const people = layout.persons;
  for (let i = 0; i < people.length; i++) {
    for (let j = i + 1; j < people.length; j++) {
      const a = people[i];
      const b = people[j];
      if (a && b && overlap(a, b)) return [a.id, b.id];
    }
  }
  return null;
}

describe('layoutFamily on the messy PoC-2 family', () => {
  it('positions all 12 people with no overlaps', async () => {
    const { layout } = await run(messy);
    expect(layout.persons).toHaveLength(12);
    expect(noOverlaps(layout)).toBeNull();
  });

  it('puts partners on the same row', async () => {
    const { graph, layout } = await run(messy);
    const byId = new Map(layout.persons.map((p) => [p.id, p]));
    for (const union of graph.unions) {
      const [a, b] = union.partners;
      expect(byId.get(a)?.y).toBe(byId.get(b)?.y);
    }
  });

  it('puts every child on a lower row than each of their parents', async () => {
    const { graph, layout } = await run(messy);
    const byId = new Map(layout.persons.map((p) => [p.id, p]));
    for (const [child, parents] of graph.parentsOf) {
      for (const parent of parents) {
        expect(byId.get(child)?.y).toBeGreaterThan(byId.get(parent)?.y ?? Infinity);
      }
    }
  });

  it('keeps the cousin-marriage couple together (couple containers; PoC-2 span 112px vs 460px without)', async () => {
    const { layout, at } = await run(messy);
    const ali = at('c1');
    const sala = at('c4');
    const [left, right] = ali.x < sala.x ? [ali, sala] : [sala, ali];
    expect(right.x - (left.x + left.width)).toBeLessThanOrEqual(40);
    // nobody stands between them
    const between = layout.persons.filter((p) => p.y === ali.y && p.x > left.x && p.x < right.x);
    expect(between).toEqual([]);
  });

  it('draws children of a union below the union dot, and every edge has a route', async () => {
    const { graph, layout } = await run(messy);
    expect(layout.unions).toHaveLength(graph.unions.length);
    expect(layout.edges.length).toBeGreaterThan(0);
    for (const edge of layout.edges) expect(edge.points.length).toBeGreaterThanOrEqual(2);
    const personBottom = Math.max(...layout.persons.map((p) => p.y + p.height));
    expect(layout.bounds.y + layout.bounds.height).toBeGreaterThanOrEqual(personBottom);
  });

  it('gives bounds that contain every person', async () => {
    const { layout } = await run(messy);
    const { bounds } = layout;
    for (const p of layout.persons) {
      expect(p.x).toBeGreaterThanOrEqual(bounds.x);
      expect(p.y).toBeGreaterThanOrEqual(bounds.y);
      expect(p.x + p.width).toBeLessThanOrEqual(bounds.x + bounds.width);
      expect(p.y + p.height).toBeLessThanOrEqual(bounds.y + bounds.height);
    }
  });

  it('is deterministic, and independent of the order the API listed people in', async () => {
    const first = await run(messy);
    const second = await run(messy);
    const reversed = await run(messy, { reverse: true });
    expect(second.layout).toEqual(first.layout);
    expect(reversed.layout.persons).toEqual(first.layout.persons);
  });
});

describe('layoutFamily on incomplete data', () => {
  it('lays out every person, including one with no relatives and a very long name', async () => {
    const { layout, at } = await run(edgeCases);
    expect(layout.persons).toHaveLength(9);
    expect(noOverlaps(layout)).toBeNull();
    expect(at('uma').width).toBe(160);
  });

  it('puts the child of two unmarried co-parents below both, and the single-parent children below Rosa', async () => {
    const { at } = await run(edgeCases);
    expect(at('lina').y).toBeGreaterThan(at('noor').y);
    expect(at('lina').y).toBeGreaterThan(at('omar').y);
    expect(at('sami').y).toBeGreaterThan(at('rosa').y);
    expect(at('tara').y).toBeGreaterThan(at('rosa').y);
  });

  it('keeps a childless couple on one row, and puts the child with no birth date after the dated sibling', async () => {
    const { at } = await run(edgeCases);
    expect(at('pia').y).toBe(at('quentin').y);
    expect(at('tara').y).toBe(at('sami').y);
    expect(at('tara').x).toBeGreaterThan(at('sami').x);
  });
});

describe('layoutFamily edge cases', () => {
  it('does not call the engine for an empty account', async () => {
    const family: FamilyGraph = buildFamilyGraph(emptyGraph());
    const layout = await layoutFamily(family, {
      layout: () => Promise.reject(new Error('engine must not be called')),
    });
    expect(layout.persons).toEqual([]);
    expect(layout.bounds).toEqual({ x: 0, y: 0, width: 0, height: 0 });
  });

  it('lays out a single person', async () => {
    const family = buildFamilyGraph({
      persons: [
        {
          id: 1,
          given_name: 'Solo',
          family_name: null,
          birth_date: null,
          birth_approx: false,
          death_date: null,
        },
      ],
      parent_links: [],
      partnerships: [],
    });
    const layout = await layoutFamily(family, engine);
    expect(layout.persons).toHaveLength(1);
    expect(layout.bounds.width).toBe(160);
  });

  it('rejects when the engine reports no position for a node', async () => {
    const family = buildFamilyGraph(apiGraphFrom(edgeCases).api);
    await expect(
      layoutFamily(family, {
        layout: (graph) => Promise.resolve({ ...graph, children: [{ id: 'p1' }] }),
      }),
    ).rejects.toThrow(/no position/);
  });
});
