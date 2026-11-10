import { memo, useImperativeHandle, useMemo, useRef, useState } from 'react';
import type { FamilyGraph } from '../domain/family';
import type { Layout } from '../layout/types';
import styles from './GraphView.module.css';
import { neighbour, type Direction } from './neighbours';
import { GRAPH_CONTROLS_ATTRIBUTE, isFromGraphControl } from './dom';
import { PersonNode } from './PersonNode';
import { useViewportControls } from './useViewportControls';

const ZOOM_STEP = 1.25;
const KEY_PAN_PX = 80;

const ARROWS: Record<string, Direction> = {
  ArrowUp: 'up',
  ArrowDown: 'down',
  ArrowLeft: 'left',
  ArrowRight: 'right',
};
// Shift+arrow pans the view; the content moves the opposite way to the camera.
const PAN_DELTA: Record<Direction, [number, number]> = {
  up: [0, KEY_PAN_PX],
  down: [0, -KEY_PAN_PX],
  left: [KEY_PAN_PX, 0],
  right: [-KEY_PAN_PX, 0],
};

function personIdFrom(target: EventTarget): number | null {
  if (!(target instanceof Element)) return null;
  const raw = target.closest('[data-person-id]')?.getAttribute('data-person-id');
  const id = raw === null || raw === undefined ? NaN : Number(raw);
  return Number.isInteger(id) ? id : null;
}

/** Edges and union dots do not change when someone is selected, so they are drawn once per layout. */
const Connections = memo(function Connections({ layout }: { layout: Layout }) {
  return (
    <>
      {layout.edges.map((edge) => (
        <polyline
          key={edge.id}
          className={edge.kind === 'partner' ? styles.partnerEdge : styles.descentEdge}
          points={edge.points.map((p) => `${String(p.x)},${String(p.y)}`).join(' ')}
        />
      ))}
      {layout.unions.map((union) => (
        <circle
          key={union.id}
          className={styles.union}
          cx={union.x + union.width / 2}
          cy={union.y + union.height / 2}
          r={4}
        />
      ))}
    </>
  );
});

/** What the screen around the graph can ask it to do. */
export interface GraphViewHandle {
  centerOn: (id: number) => void;
  focusPerson: (id: number) => void;
}

interface Props {
  layout: Layout;
  family: FamilyGraph;
  selectedId: number | null;
  onSelect: (id: number | null) => void;
  ref?: React.Ref<GraphViewHandle>;
}

/**
 * Draws a layout as SVG and handles pan, zoom, selection and the keyboard. It only draws the
 * coordinates it is given; the domain data is used for labels and for moving between relatives.
 */
export function GraphView({ layout, family, selectedId, onSelect, ref }: Props) {
  const boxes = useMemo(() => new Map(layout.persons.map((p) => [p.id, p])), [layout]);
  const earliest = family.persons[0];
  const firstBox = earliest ? boxes.get(earliest.id) : undefined;
  const containerRef = useRef<HTMLDivElement>(null);
  const groupRef = useRef<SVGGElement>(null);
  const controls = useViewportControls(containerRef, groupRef, layout, firstBox);

  // Which person is the tab stop. It follows real DOM focus (see onFocus), never the other way round.
  const [focusId, setFocusId] = useState<number | null>(null);

  // Roving tabindex: the graph is one tab stop, on the focused person, else the selected one, else the eldest.
  const tabbableId =
    focusId !== null && boxes.has(focusId)
      ? focusId
      : selectedId !== null && boxes.has(selectedId)
        ? selectedId
        : (earliest?.id ?? null);

  /** Every person is always in the DOM, so focus can move straight to them. */
  function focusPerson(id: number) {
    containerRef.current
      ?.querySelector<SVGGElement>(`[data-person-id="${String(id)}"]`)
      ?.focus({ preventScroll: true });
  }

  useImperativeHandle(ref, () => ({
    centerOn(id) {
      const box = boxes.get(id);
      if (box) controls.centerOnRect(box);
    },
    focusPerson,
  }));

  function select(id: number) {
    setFocusId(id);
    onSelect(id);
  }

  function onClick(event: React.MouseEvent) {
    if (controls.wasDragged() || isFromGraphControl(event.target)) return;
    const id = personIdFrom(event.target);
    if (id === null || id === selectedId) onSelect(null);
    else select(id);
  }

  function onFocus(event: React.FocusEvent) {
    const id = personIdFrom(event.target);
    if (id !== null) setFocusId(id);
  }

  function onDoubleClick(event: React.MouseEvent) {
    const id = personIdFrom(event.target);
    const box = id === null ? undefined : boxes.get(id);
    if (id !== null && box) {
      select(id);
      controls.centerOnRect(box);
    }
  }

  function onKeyDown(event: React.KeyboardEvent) {
    if (event.ctrlKey || event.metaKey || event.altKey) return;
    const id = personIdFrom(event.target);
    const arrow = ARROWS[event.key];

    if (event.key === '+' || event.key === '=') {
      event.preventDefault();
      controls.zoomBy(ZOOM_STEP);
    } else if (event.key === '-' || event.key === '_') {
      event.preventDefault();
      controls.zoomBy(1 / ZOOM_STEP);
    } else if (event.key === '0') {
      event.preventDefault();
      controls.fit();
    } else if (event.key === 'Escape') {
      onSelect(null);
    } else if (arrow !== undefined && event.shiftKey) {
      event.preventDefault();
      const [dx, dy] = PAN_DELTA[arrow];
      controls.pan(dx, dy);
    } else if (id !== null && (event.key === 'Enter' || event.key === ' ')) {
      event.preventDefault();
      if (id === selectedId) onSelect(null);
      else select(id);
    } else if (id !== null && arrow !== undefined) {
      event.preventDefault();
      const next = neighbour(layout, family, id, arrow);
      const nextBox = next === null ? undefined : boxes.get(next);
      if (next !== null && nextBox) {
        focusPerson(next);
        if (!controls.isVisible(nextBox)) controls.centerOnRect(nextBox);
      }
    }
  }

  return (
    <div
      ref={containerRef}
      className={styles.viewport}
      {...controls.pointerHandlers}
      onClick={onClick}
      onDoubleClick={onDoubleClick}
      onKeyDown={onKeyDown}
      onFocus={onFocus}
    >
      <svg className={styles.svg} role="group" aria-label="Family graph">
        <g ref={groupRef} className={styles.world}>
          <Connections layout={layout} />
          {layout.persons.map((box) => {
            const person = family.personById.get(box.id);
            return person ? (
              <PersonNode
                key={box.id}
                person={person}
                box={box}
                selected={box.id === selectedId}
                tabbable={box.id === tabbableId}
              />
            ) : null;
          })}
        </g>
      </svg>

      <div
        className={styles.toolbar}
        role="toolbar"
        aria-label="Graph controls"
        {...{ [GRAPH_CONTROLS_ATTRIBUTE]: true }}
      >
        <button
          type="button"
          aria-label="Zoom in"
          onClick={() => {
            controls.zoomBy(ZOOM_STEP);
          }}
        >
          +
        </button>
        <button
          type="button"
          aria-label="Zoom out"
          onClick={() => {
            controls.zoomBy(1 / ZOOM_STEP);
          }}
        >
          −
        </button>
        <button type="button" onClick={controls.fit}>
          Fit
        </button>
      </div>
    </div>
  );
}
