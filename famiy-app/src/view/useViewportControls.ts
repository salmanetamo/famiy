import { useCallback, useEffect, useLayoutEffect, useRef, type RefObject } from 'react';
import type { Layout, Rect } from '../layout/types';
import { isFromGraphControl } from './dom';
import {
  centerOn,
  fitViewport,
  initialViewport,
  panBy,
  wheelDeltaPixels,
  zoomAt,
  type Size,
  type Viewport,
} from './viewport';

const DRAG_THRESHOLD_PX = 4;
const ANIMATION_MS = 250;
const WHEEL_SENSITIVITY = 0.0015;

const prefersReducedMotion = (): boolean =>
  typeof window.matchMedia === 'function' &&
  window.matchMedia('(prefers-reduced-motion: reduce)').matches;

interface Drag {
  pointerId: number;
  lastX: number;
  lastY: number;
  startX: number;
  startY: number;
  active: boolean;
}

/**
 * Owns the pan/zoom view state. The view lives in a ref and is written straight to the SVG group's
 * CSS transform (ADR-002 ladder step 1), so panning never re-renders the people. It is view state
 * only: nothing here knows about families.
 */
export function useViewportControls(
  containerRef: RefObject<HTMLDivElement | null>,
  groupRef: RefObject<SVGGElement | null>,
  layout: Layout,
  firstPerson: Rect | undefined,
  /** Told about every change of the view (pan, zoom, fit), with the view and the container size. */
  onViewChange?: (view: Viewport, size: Size) => void,
) {
  const view = useRef<Viewport>({ x: 0, y: 0, k: 1 });
  const animation = useRef<number | null>(null);
  const drag = useRef<Drag | null>(null);
  const dragged = useRef(false);
  const initialised = useRef(false);

  const size = useCallback((): Size => {
    const box = containerRef.current?.getBoundingClientRect();
    // jsdom has no layout (zero size), so fall back to a plausible size there
    const width = box?.width ?? 0;
    const height = box?.height ?? 0;
    return { width: width > 0 ? width : 800, height: height > 0 ? height : 600 };
  }, [containerRef]);

  // Declared before the first layout effect below, which applies the opening view and so notifies.
  const onViewChangeRef = useRef(onViewChange);
  useLayoutEffect(() => {
    onViewChangeRef.current = onViewChange;
  });

  const apply = useCallback(
    (next: Viewport) => {
      view.current = next;
      groupRef.current?.style.setProperty(
        'transform',
        `translate(${String(next.x)}px, ${String(next.y)}px) scale(${String(next.k)})`,
      );
      onViewChangeRef.current?.(next, size());
    },
    [groupRef, size],
  );

  const cancelAnimation = useCallback(() => {
    if (animation.current !== null) cancelAnimationFrame(animation.current);
    animation.current = null;
  }, []);

  const moveTo = useCallback(
    (target: Viewport, animate: boolean) => {
      cancelAnimation();
      if (!animate || prefersReducedMotion()) {
        apply(target);
        return;
      }
      const from = view.current;
      const start = performance.now();
      const step = () => {
        // Same clock as `start` (the timestamp rAF passes in can be earlier than `start`, or on a
        // different origin), and clamped so the easing can never overshoot.
        const t = Math.min(1, Math.max(0, (performance.now() - start) / ANIMATION_MS));
        const eased = 1 - Math.pow(1 - t, 3);
        apply({
          x: from.x + (target.x - from.x) * eased,
          y: from.y + (target.y - from.y) * eased,
          k: from.k + (target.k - from.k) * eased,
        });
        animation.current = t < 1 ? requestAnimationFrame(step) : null;
      };
      animation.current = requestAnimationFrame(step);
    },
    [apply, cancelAnimation],
  );

  // First layout only: later layouts (after a refetch) must not make the view jump.
  useLayoutEffect(() => {
    if (initialised.current || layout.persons.length === 0) return;
    initialised.current = true;
    apply(initialViewport(layout, size(), firstPerson));
  }, [layout, firstPerson, apply, size]);

  // A non-passive native listener, so the page does not scroll while zooming the graph.
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const onWheel = (event: WheelEvent) => {
      event.preventDefault();
      cancelAnimation();
      const box = el.getBoundingClientRect();
      const cursor = { x: event.clientX - box.left, y: event.clientY - box.top };
      const pixels = wheelDeltaPixels(event.deltaY, event.deltaMode);
      apply(zoomAt(view.current, cursor, Math.exp(-pixels * WHEEL_SENSITIVITY)));
    };
    el.addEventListener('wheel', onWheel, { passive: false });
    return () => {
      el.removeEventListener('wheel', onWheel);
    };
  }, [apply, cancelAnimation, containerRef]);

  useEffect(() => cancelAnimation, [cancelAnimation]);

  const zoomBy = useCallback(
    (factor: number) => {
      const { width, height } = size();
      moveTo(zoomAt(view.current, { x: width / 2, y: height / 2 }, factor), false);
    },
    [moveTo, size],
  );

  const fit = useCallback(() => {
    moveTo(fitViewport(layout.bounds, size()), true);
  }, [layout, moveTo, size]);

  const centerOnRect = useCallback(
    (rect: Rect) => {
      moveTo(
        centerOn(view.current, { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 }, size()),
        true,
      );
    },
    [moveTo, size],
  );

  const pan = useCallback(
    (dx: number, dy: number) => {
      cancelAnimation();
      apply(panBy(view.current, dx, dy));
    },
    [apply, cancelAnimation],
  );

  /** True when the rectangle (world coordinates) is fully inside the current view. */
  const isVisible = useCallback(
    (rect: Rect): boolean => {
      const { width, height } = size();
      const v = view.current;
      const left = rect.x * v.k + v.x;
      const top = rect.y * v.k + v.y;
      return (
        left >= 0 &&
        top >= 0 &&
        left + rect.width * v.k <= width &&
        top + rect.height * v.k <= height
      );
    },
    [size],
  );

  const endDrag = () => {
    drag.current = null;
    // The click that follows a drag must not select or deselect anything.
    if (dragged.current) setTimeout(() => (dragged.current = false), 0);
  };

  /** Tell the owner about the current view again (the container was resized). */
  const notify = useCallback(() => {
    onViewChangeRef.current?.(view.current, size());
  }, [size]);

  const pointerHandlers = {
    onPointerDown: (event: React.PointerEvent<HTMLDivElement>) => {
      if (event.button !== 0 || isFromGraphControl(event.target)) return;
      cancelAnimation();
      drag.current = {
        pointerId: event.pointerId,
        lastX: event.clientX,
        lastY: event.clientY,
        startX: event.clientX,
        startY: event.clientY,
        active: false,
      };
    },
    onPointerMove: (event: React.PointerEvent<HTMLDivElement>) => {
      const d = drag.current;
      if (d?.pointerId !== event.pointerId) return;
      // A mouse released outside the window sends no pointerup; if it comes back with no button
      // held, the drag is over (otherwise the graph would follow a mouse that is not pressed).
      if (event.pointerType === 'mouse' && event.buttons === 0) {
        endDrag();
        return;
      }
      if (!d.active) {
        const far =
          Math.hypot(event.clientX - d.startX, event.clientY - d.startY) >= DRAG_THRESHOLD_PX;
        if (!far) return;
        d.active = true;
        dragged.current = true;
        // Capture only once it is a drag, so a plain click still reaches the person it landed on.
        event.currentTarget.setPointerCapture(event.pointerId);
      }
      apply(panBy(view.current, event.clientX - d.lastX, event.clientY - d.lastY));
      d.lastX = event.clientX;
      d.lastY = event.clientY;
    },
    onPointerUp: (event: React.PointerEvent<HTMLDivElement>) => {
      if (drag.current?.pointerId !== event.pointerId) return;
      endDrag();
    },
    onLostPointerCapture: (event: React.PointerEvent<HTMLDivElement>) => {
      if (drag.current?.pointerId !== event.pointerId) return;
      endDrag();
    },
  };

  return {
    pointerHandlers: { ...pointerHandlers, onPointerCancel: pointerHandlers.onPointerUp },
    zoomBy,
    fit,
    pan,
    centerOnRect,
    isVisible,
    notify,
    wasDragged: () => dragged.current,
  };
}
