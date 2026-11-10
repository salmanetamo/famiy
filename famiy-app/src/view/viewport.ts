import type { Layout, Point, Rect } from '../layout/types';

/** View state, kept apart from the domain (ADR-002): a screen point is `world * k + (x, y)`. */
export interface Viewport {
  x: number;
  y: number;
  k: number;
}

export interface Size {
  width: number;
  height: number;
}

export const MIN_ZOOM = 0.05;
export const MAX_ZOOM = 3;
/** Below this scale a fitted graph is too small to read, so the initial view zooms in instead. */
export const READABLE_ZOOM = 0.5;

export const clampZoom = (k: number): number => Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, k));

export const toScreen = (v: Viewport, world: Point): Point => ({
  x: world.x * v.k + v.x,
  y: world.y * v.k + v.y,
});

export const toWorld = (v: Viewport, screen: Point): Point => ({
  x: (screen.x - v.x) / v.k,
  y: (screen.y - v.y) / v.k,
});

/** Zooms by `factor` while the world point under `screen` stays under it (zoom about the cursor). */
export function zoomAt(v: Viewport, screen: Point, factor: number): Viewport {
  const k = clampZoom(v.k * factor);
  const world = toWorld(v, screen);
  return { k, x: screen.x - world.x * k, y: screen.y - world.y * k };
}

/**
 * A wheel's vertical delta in pixels. Firefox reports a mouse wheel in lines (deltaMode 1) and some
 * devices in pages (2); treating those as pixels would make zooming about 30 times too slow.
 */
export function wheelDeltaPixels(deltaY: number, deltaMode: number): number {
  if (deltaMode === 1) return deltaY * 16;
  if (deltaMode === 2) return deltaY * 100;
  return deltaY;
}

export const panBy = (v: Viewport, dx: number, dy: number): Viewport => ({
  ...v,
  x: v.x + dx,
  y: v.y + dy,
});

/** Puts the world point at the middle of the view, optionally at a new zoom. */
export function centerOn(v: Viewport, world: Point, size: Size, k: number = v.k): Viewport {
  const zoom = clampZoom(k);
  return { k: zoom, x: size.width / 2 - world.x * zoom, y: size.height / 2 - world.y * zoom };
}

/** The viewport that shows all of `bounds` with `padding`, never zoomed in past `maxZoom`. */
export function fitViewport(bounds: Rect, size: Size, padding = 40, maxZoom = 1): Viewport {
  if (bounds.width <= 0 || bounds.height <= 0) {
    return { x: size.width / 2, y: size.height / 2, k: 1 };
  }
  const k = clampZoom(
    Math.min(
      (size.width - 2 * padding) / bounds.width,
      (size.height - 2 * padding) / bounds.height,
      maxZoom,
    ),
  );
  return centerOn(
    { x: 0, y: 0, k },
    { x: bounds.x + bounds.width / 2, y: bounds.y + bounds.height / 2 },
    size,
    k,
  );
}

/**
 * The first view of a layout: everything in view if that is still readable, otherwise normal size
 * centered on `focus` (the earliest-born person), because a fitted graph of thousands is a smudge.
 */
export function initialViewport(layout: Layout, size: Size, focus: Rect | undefined): Viewport {
  const fit = fitViewport(layout.bounds, size);
  if (fit.k >= READABLE_ZOOM || !focus) return fit;
  return centerOn(fit, { x: focus.x + focus.width / 2, y: focus.y + focus.height / 2 }, size, 1);
}
