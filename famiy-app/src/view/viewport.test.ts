import { describe, expect, it } from 'vitest';
import type { Layout } from '../layout/types';
import {
  centerOn,
  clampZoom,
  fitViewport,
  initialViewport,
  MAX_ZOOM,
  MIN_ZOOM,
  panBy,
  toScreen,
  toWorld,
  wheelDeltaPixels,
  zoomAt,
} from './viewport';

const size = { width: 800, height: 600 };

describe('zoomAt', () => {
  it('keeps the world point under the cursor fixed', () => {
    const v = { x: 50, y: -20, k: 1.5 };
    const cursor = { x: 300, y: 200 };
    const before = toWorld(v, cursor);
    const after = zoomAt(v, cursor, 1.3);
    const again = toWorld(after, cursor);
    expect(after.k).toBeCloseTo(1.95);
    expect(again.x).toBeCloseTo(before.x);
    expect(again.y).toBeCloseTo(before.y);
  });

  it('never zooms past the limits, and still pins the cursor when clamped', () => {
    const v = { x: 0, y: 0, k: MAX_ZOOM };
    const capped = zoomAt(v, { x: 100, y: 100 }, 10);
    expect(capped.k).toBe(MAX_ZOOM);
    expect(toWorld(capped, { x: 100, y: 100 })).toEqual(toWorld(v, { x: 100, y: 100 }));
    expect(zoomAt({ x: 0, y: 0, k: MIN_ZOOM }, { x: 0, y: 0 }, 0.01).k).toBe(MIN_ZOOM);
    expect(clampZoom(0)).toBe(MIN_ZOOM);
  });
});

describe('toScreen / toWorld', () => {
  it('are inverses', () => {
    const v = { x: 12, y: 34, k: 2.5 };
    const p = { x: 7, y: -9 };
    const back = toWorld(v, toScreen(v, p));
    expect(back.x).toBeCloseTo(p.x);
    expect(back.y).toBeCloseTo(p.y);
  });
});

describe('panBy', () => {
  it('moves the view without changing the zoom', () => {
    expect(panBy({ x: 1, y: 2, k: 3 }, 10, -5)).toEqual({ x: 11, y: -3, k: 3 });
  });
});

describe('centerOn', () => {
  it('puts the world point in the middle of the view', () => {
    const v = centerOn({ x: 0, y: 0, k: 2 }, { x: 100, y: 50 }, size);
    expect(toScreen(v, { x: 100, y: 50 })).toEqual({ x: 400, y: 300 });
    expect(v.k).toBe(2);
  });

  it('can change the zoom at the same time', () => {
    const v = centerOn({ x: 0, y: 0, k: 0.3 }, { x: 10, y: 10 }, size, 1);
    expect(v.k).toBe(1);
    expect(toScreen(v, { x: 10, y: 10 })).toEqual({ x: 400, y: 300 });
  });
});

describe('fitViewport', () => {
  it('fits a large area inside the view with padding, centered', () => {
    const bounds = { x: 0, y: 0, width: 1600, height: 400 };
    const v = fitViewport(bounds, size);
    expect(v.k).toBeCloseTo((800 - 80) / 1600);
    const topLeft = toScreen(v, { x: 0, y: 0 });
    const bottomRight = toScreen(v, { x: 1600, y: 400 });
    expect(topLeft.x).toBeGreaterThanOrEqual(40 - 1e-9);
    expect(bottomRight.x).toBeLessThanOrEqual(760 + 1e-9);
    expect((topLeft.y + bottomRight.y) / 2).toBeCloseTo(300);
  });

  it('does not zoom in past 1 for a small graph', () => {
    expect(fitViewport({ x: 0, y: 0, width: 100, height: 100 }, size).k).toBe(1);
  });

  it('copes with empty bounds', () => {
    expect(fitViewport({ x: 0, y: 0, width: 0, height: 0 }, size)).toEqual({
      x: 400,
      y: 300,
      k: 1,
    });
  });
});

describe('initialViewport', () => {
  const layoutOf = (width: number, height: number): Layout => ({
    persons: [],
    unions: [],
    edges: [],
    bounds: { x: 0, y: 0, width, height },
  });

  it('fits everything when that is still readable', () => {
    const v = initialViewport(layoutOf(1000, 500), size, { x: 0, y: 0, width: 160, height: 56 });
    expect(v.k).toBeGreaterThanOrEqual(0.5);
    expect(v).toEqual(fitViewport({ x: 0, y: 0, width: 1000, height: 500 }, size));
  });

  it('zooms to normal size on the focus person when fitting would be unreadable', () => {
    const focus = { x: 100, y: 200, width: 160, height: 56 };
    const v = initialViewport(layoutOf(20000, 8000), size, focus);
    expect(v.k).toBe(1);
    expect(toScreen(v, { x: 180, y: 228 })).toEqual({ x: 400, y: 300 });
  });

  it('falls back to the fit when there is nobody to focus on', () => {
    const v = initialViewport(layoutOf(20000, 8000), size, undefined);
    expect(v.k).toBeLessThan(0.5);
  });
});

describe('wheelDeltaPixels', () => {
  it('leaves pixel deltas alone and scales line and page deltas up to pixels', () => {
    expect(wheelDeltaPixels(-120, 0)).toBe(-120);
    expect(wheelDeltaPixels(-3, 1)).toBe(-48);
    expect(wheelDeltaPixels(2, 2)).toBe(200);
  });
});
