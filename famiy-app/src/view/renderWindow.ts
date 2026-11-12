import type { Layout, PositionedPerson, PositionedUnion, Rect, RoutedEdge } from '../layout/types';
import type { Size, Viewport } from './viewport';

/*
 * Viewport culling (ADR-002 ladder step 2). Drawing every person is what makes zoom slow in a big
 * tree, so only the people, unions and edges inside a window around the view are drawn. Layout is
 * untouched. The window is the view plus one view of margin on every side, and it is replaced only
 * when the view leaves it (or has shrunk far inside it), so panning does not re-render per frame.
 */

/** Below this scale the text on a card cannot be read, so cards are drawn as bare rectangles. */
export const COMPACT_BELOW = 0.3;
/** Back to full cards only above this, so a view hovering at the threshold does not flicker. */
export const COMPACT_ABOVE = 0.36;
/** A window this many times larger than the view (after zooming in) is replaced by a tighter one. */
export const MAX_AREA_RATIO = 20;

export interface RenderWindow {
  /** World coordinates. */
  rect: Rect;
  /** Draw cards as bare rectangles (no text), because the scale is too small to read them. */
  compact: boolean;
}

/** The part of the world the view shows, in world coordinates. */
export function viewportWorldRect(view: Viewport, size: Size): Rect {
  return {
    x: -view.x / view.k,
    y: -view.y / view.k,
    width: size.width / view.k,
    height: size.height / view.k,
  };
}

/** `rect` grown by `factor` times its own width and height on every side. */
export function expandRect(rect: Rect, factor: number): Rect {
  return {
    x: rect.x - rect.width * factor,
    y: rect.y - rect.height * factor,
    width: rect.width * (1 + 2 * factor),
    height: rect.height * (1 + 2 * factor),
  };
}

export const containsRect = (outer: Rect, inner: Rect): boolean =>
  inner.x >= outer.x &&
  inner.y >= outer.y &&
  inner.x + inner.width <= outer.x + outer.width &&
  inner.y + inner.height <= outer.y + outer.height;

export const intersects = (a: Rect, b: Rect): boolean =>
  a.x <= b.x + b.width && a.x + a.width >= b.x && a.y <= b.y + b.height && a.y + a.height >= b.y;

export const nextCompact = (previous: boolean, scale: number): boolean =>
  previous ? scale < COMPACT_ABOVE : scale < COMPACT_BELOW;

/** True when the current window no longer suits the view, so a new one should be made. */
export function windowNeedsUpdate(
  current: RenderWindow | null,
  viewRect: Rect,
  scale: number,
): boolean {
  if (current === null) return true;
  if (!containsRect(current.rect, viewRect)) return true;
  if (
    current.rect.width * current.rect.height >
    MAX_AREA_RATIO * viewRect.width * viewRect.height
  ) {
    return true;
  }
  return nextCompact(current.compact, scale) !== current.compact;
}

export function makeWindow(viewRect: Rect, scale: number, previousCompact: boolean): RenderWindow {
  return { rect: expandRect(viewRect, 1), compact: nextCompact(previousCompact, scale) };
}

export interface Visible {
  persons: readonly PositionedPerson[];
  unions: readonly PositionedUnion[];
  edges: readonly RoutedEdge[];
}

function edgeBounds(edge: RoutedEdge): Rect {
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const point of edge.points) {
    minX = Math.min(minX, point.x);
    minY = Math.min(minY, point.y);
    maxX = Math.max(maxX, point.x);
    maxY = Math.max(maxY, point.y);
  }
  return { x: minX, y: minY, width: maxX - minX, height: maxY - minY };
}

/**
 * What lies inside `rect`. A plain scan: at 10,000 people it takes well under a millisecond, which
 * is why there is no spatial index. An edge counts if its bounding box touches the rectangle, so an
 * edge between a drawn person and one outside is still drawn.
 */
export function visibleSet(layout: Layout, rect: Rect): Visible {
  return {
    persons: layout.persons.filter((person) => intersects(person, rect)),
    unions: layout.unions.filter((union) => intersects(union, rect)),
    edges: layout.edges.filter(
      (edge) => edge.points.length > 0 && intersects(edgeBounds(edge), rect),
    ),
  };
}
