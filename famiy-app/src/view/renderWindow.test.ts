import { describe, expect, it } from 'vitest';
import type { Layout } from '../layout/types';
import {
  COMPACT_ABOVE,
  COMPACT_BELOW,
  containsRect,
  expandRect,
  intersects,
  makeWindow,
  MAX_AREA_RATIO,
  nextCompact,
  viewportWorldRect,
  visibleSet,
  windowNeedsUpdate,
  type RenderWindow,
} from './renderWindow';

const rect = (x: number, y: number, width: number, height: number) => ({ x, y, width, height });

describe('viewportWorldRect', () => {
  it('is the part of the world the view shows', () => {
    // translated by (-100, -50) and zoomed 2x: the screen's top-left is world (50, 25), 400 x 300 wide
    expect(viewportWorldRect({ x: -100, y: -50, k: 2 }, { width: 800, height: 600 })).toEqual(
      rect(50, 25, 400, 300),
    );
  });

  it('grows as the view zooms out', () => {
    const near = viewportWorldRect({ x: 0, y: 0, k: 1 }, { width: 800, height: 600 });
    const far = viewportWorldRect({ x: 0, y: 0, k: 0.25 }, { width: 800, height: 600 });
    expect(far.width).toBe(near.width * 4);
  });
});

describe('expandRect and containsRect', () => {
  it('adds a margin of the rectangle’s own size on every side', () => {
    expect(expandRect(rect(100, 100, 50, 20), 1)).toEqual(rect(50, 80, 150, 60));
  });

  it('contains itself and what lies inside, and nothing that pokes out on any side', () => {
    const outer = rect(0, 0, 100, 100);
    expect(containsRect(outer, outer)).toBe(true);
    expect(containsRect(outer, rect(10, 10, 20, 20))).toBe(true);
    expect(containsRect(outer, rect(-1, 10, 20, 20))).toBe(false);
    expect(containsRect(outer, rect(10, -1, 20, 20))).toBe(false);
    expect(containsRect(outer, rect(90, 10, 20, 20))).toBe(false);
    expect(containsRect(outer, rect(10, 90, 20, 20))).toBe(false);
  });

  it('intersects when touching or overlapping, not when apart', () => {
    const a = rect(0, 0, 10, 10);
    expect(intersects(a, rect(10, 0, 10, 10))).toBe(true); // sharing an edge, on each side
    expect(intersects(a, rect(-10, 0, 10, 10))).toBe(true);
    expect(intersects(a, rect(0, 10, 10, 10))).toBe(true);
    expect(intersects(a, rect(0, -10, 10, 10))).toBe(true);
    expect(intersects(a, rect(5, 5, 10, 10))).toBe(true);
    expect(intersects(a, rect(11, 0, 10, 10))).toBe(false);
    expect(intersects(a, rect(0, 11, 10, 10))).toBe(false);
    expect(intersects(a, rect(-20, 0, 10, 10))).toBe(false);
  });
});

describe('nextCompact (with hysteresis)', () => {
  it('turns on below the lower threshold and off above the upper one', () => {
    expect(nextCompact(false, COMPACT_BELOW - 0.01)).toBe(true);
    expect(nextCompact(false, COMPACT_BELOW + 0.01)).toBe(false);
    expect(nextCompact(true, COMPACT_ABOVE + 0.01)).toBe(false);
  });

  it('keeps its state in between, so a view hovering at the threshold does not flicker', () => {
    const between = (COMPACT_BELOW + COMPACT_ABOVE) / 2;
    expect(nextCompact(false, between)).toBe(false);
    expect(nextCompact(true, between)).toBe(true);
  });
});

describe('windowNeedsUpdate', () => {
  const view = rect(1000, 1000, 800, 600);
  const window: RenderWindow = makeWindow(view, 1, false);

  it('needs a window when there is none', () => {
    expect(windowNeedsUpdate(null, view, 1)).toBe(true);
  });

  it('keeps the window while the view stays inside it, even after a small pan', () => {
    expect(windowNeedsUpdate(window, view, 1)).toBe(false);
    expect(windowNeedsUpdate(window, rect(1500, 1300, 800, 600), 1)).toBe(false);
  });

  it('makes a new window once the view reaches past its edge', () => {
    expect(windowNeedsUpdate(window, rect(2100, 1000, 800, 600), 1)).toBe(true);
    expect(windowNeedsUpdate(window, rect(1000, 100, 800, 600), 1)).toBe(true);
  });

  it('makes a new window when zooming out has outgrown it', () => {
    expect(windowNeedsUpdate(window, rect(800, 700, 2400, 1800), 0.33)).toBe(true);
  });

  it('replaces a window that is far larger than a zoomed-in view, and keeps one that is only a bit larger', () => {
    const tiny = rect(1100, 1100, 80, 60);
    expect(window.rect.width * window.rect.height).toBeGreaterThan(
      MAX_AREA_RATIO * tiny.width * tiny.height,
    );
    expect(windowNeedsUpdate(window, tiny, 10)).toBe(true);
    // 600 x 450 is a quarter smaller on each side: the window is 16 times its area, under the limit
    const modest = rect(1100, 1100, 600, 450);
    expect(windowNeedsUpdate(window, modest, 1.33)).toBe(false);
  });

  it('makes a new window when the cards should switch between full and compact', () => {
    const full = makeWindow(view, 1, false);
    expect(full.compact).toBe(false);
    expect(windowNeedsUpdate(full, view, COMPACT_BELOW - 0.05)).toBe(true);
    const compact = makeWindow(view, COMPACT_BELOW - 0.05, false);
    expect(compact.compact).toBe(true);
    expect(windowNeedsUpdate(compact, view, COMPACT_ABOVE + 0.05)).toBe(true);
    // inside the hysteresis band neither switches
    const between = (COMPACT_BELOW + COMPACT_ABOVE) / 2;
    expect(windowNeedsUpdate(full, view, between)).toBe(false);
    expect(windowNeedsUpdate(compact, view, between)).toBe(false);
  });
});

describe('visibleSet', () => {
  const layout: Layout = {
    persons: [
      { id: 1, ...rect(0, 0, 160, 56) },
      { id: 2, ...rect(500, 0, 160, 56) }, // straddles the window's right edge
      { id: 3, ...rect(5000, 0, 160, 56) },
    ],
    unions: [
      { id: '1-2', kind: null, ...rect(300, 20, 14, 14) },
      { id: '8-9', kind: null, ...rect(5000, 20, 14, 14) },
    ],
    edges: [
      {
        id: 'inside',
        kind: 'descent',
        points: [
          { x: 10, y: 10 },
          { x: 50, y: 60 },
        ],
      },
      // both ends are outside the window, on opposite sides, but the line crosses it
      {
        id: 'crossing',
        kind: 'descent',
        points: [
          { x: -900, y: 30 },
          { x: 900, y: 30 },
        ],
      },
      {
        id: 'outside',
        kind: 'descent',
        points: [
          { x: 5000, y: 0 },
          { x: 5100, y: 50 },
        ],
      },
      { id: 'empty', kind: 'descent', points: [] },
    ],
    bounds: rect(0, 0, 5200, 60),
  };
  const window = rect(-100, -100, 700, 400);

  it('keeps the people and unions that touch the window and drops the rest', () => {
    const seen = visibleSet(layout, window);
    expect(seen.persons.map((p) => p.id)).toEqual([1, 2]);
    expect(seen.unions.map((u) => u.id)).toEqual(['1-2']);
  });

  it('keeps an edge whose bounding box touches the window, even if neither end is inside', () => {
    expect(visibleSet(layout, window).edges.map((e) => e.id)).toEqual(['inside', 'crossing']);
  });

  it('keeps everything when the window covers the whole layout', () => {
    const all = visibleSet(layout, rect(-1000, -1000, 7000, 2000));
    expect(all.persons).toHaveLength(3);
    expect(all.edges.map((e) => e.id)).toEqual(['inside', 'crossing', 'outside']);
  });
});
