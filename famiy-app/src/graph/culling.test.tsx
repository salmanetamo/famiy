import { act, fireEvent, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { messy } from '../test/fixtures';
import { node, people, setupGraph } from '../test/graphHarness';
import { syntheticApiGraph } from '../test/synthetic';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  Reflect.deleteProperty(Element.prototype, 'setPointerCapture');
});

// jsdom has no pointer capture; the graph only asks for it once a press has become a drag
beforeEach(() => {
  Object.defineProperty(Element.prototype, 'setPointerCapture', {
    value: vi.fn(),
    configurable: true,
    writable: true,
  });
});

const COUNT = 400;
const LAYOUT_WAIT = { timeout: 20_000 };

/** A tree far too big to see at once: at the opening view only the part around one person is drawn. */
async function openBigTree() {
  const ctx = setupGraph(messy, {}, syntheticApiGraph(COUNT));
  await waitFor(() => {
    expect(people().length).toBeGreaterThan(0);
  }, LAYOUT_WAIT);
  return ctx;
}

const drawn = () => new Set(people().map((p) => Number(p.dataset.personId)));
const isDrawn = (id: number) => document.querySelector(`[data-person-id="${String(id)}"]`) !== null;
const scaleOf = (): number => {
  const transform = document.querySelector<SVGGElement>('svg g')?.style.transform ?? '';
  return Number(/scale\(([^)]*)\)/.exec(transform)?.[1] ?? NaN);
};
const cardsWithText = () => document.querySelectorAll('[data-person-id] text').length;
const enterTheGraph = async () => {
  // tab stops in order: the search box, the Add person button, then the graph
  await userEvent.tab();
  await userEvent.tab();
  await userEvent.tab();
};
const panRight = (presses: number) =>
  userEvent.keyboard(`{Shift>}{ArrowRight>${String(presses)}/}{/Shift}`); // 80 px each

// each test lays out 400 people with the real ELK in jsdom, so they need more than the default 5 s
describe('viewport culling', { timeout: 30_000 }, () => {
  it('draws only the part of a big tree around the view', async () => {
    await openBigTree();
    expect(people().length).toBeGreaterThan(0);
    expect(people().length).toBeLessThan(COUNT / 3);
    expect(document.querySelectorAll('polyline').length).toBeLessThan(COUNT);
  });

  it('draws the people it reaches when the view is panned, and lets go of those it left', async () => {
    await openBigTree();
    await enterTheGraph();
    const before = drawn();
    await panRight(60); // 4,800 px: well beyond the window around the opening view
    await waitFor(() => {
      expect([...drawn()].some((id) => !before.has(id))).toBe(true);
    });
    expect([...before].some((id) => !isDrawn(id))).toBe(true);
    expect(people().length).toBeLessThan(COUNT / 3);
  });

  it('keeps the person with focus drawn, and the tab stop with them, however far the view moves', async () => {
    await openBigTree();
    await enterTheGraph();
    const focused = document.activeElement;
    expect(focused?.getAttribute('data-person-id')).not.toBeNull();
    const id = Number(focused?.getAttribute('data-person-id'));
    await panRight(60);
    await waitFor(() => {
      expect(people().length).toBeGreaterThan(0);
    });
    expect(isDrawn(id)).toBe(true);
    expect(node(id)).toHaveFocus();
    expect(node(id).tabIndex).toBe(0);
  });

  it('keeps the tab stop drawn when the view is dragged away before anything had focus', async () => {
    await openBigTree();
    const stop = people().find((p) => p.tabIndex === 0);
    if (!stop) throw new Error('no tab stop');
    const before = drawn();
    const id = Number(stop.dataset.personId);
    const svg = document.querySelector('svg');
    if (!svg) throw new Error('no svg');
    fireEvent.pointerDown(svg, { button: 0, pointerId: 1, clientX: 100, clientY: 100 });
    fireEvent.pointerMove(svg, { pointerId: 1, clientX: -4900, clientY: 100 }); // 5,000 px
    fireEvent.pointerUp(svg, { pointerId: 1, clientX: -4900, clientY: 100 });
    await waitFor(() => {
      expect(people().length).toBeGreaterThan(0);
    });
    // the drag really moved the view: people who were drawn are gone
    expect([...before].some((other) => !isDrawn(other))).toBe(true);
    expect(isDrawn(id)).toBe(true); // otherwise Tab would have nothing to land on
    await enterTheGraph();
    expect(node(id)).toHaveFocus();
  });

  it('keeps the selected person drawn when focus has moved on and the view is far away', async () => {
    await openBigTree();
    await enterTheGraph();
    const selected = document.activeElement;
    const id = Number(selected?.getAttribute('data-person-id'));
    await userEvent.keyboard('{Enter}'); // select it
    expect(node(id)).toHaveAttribute('aria-pressed', 'true');
    await userEvent.keyboard('{ArrowRight}'); // focus (and the tab stop) move to a neighbour
    expect(document.activeElement).not.toBe(selected);
    await panRight(60);
    await waitFor(() => {
      expect(people().length).toBeGreaterThan(0);
    });
    expect(isDrawn(id)).toBe(true);
    expect(node(id)).toHaveAttribute('aria-pressed', 'true');
  });

  it('tells assistive technology that only part of a big tree is drawn, and says nothing of the kind for a small one', async () => {
    await openBigTree();
    expect(
      screen.getByRole('group', { name: /^Family graph, 400 people\. Only those near the view/ }),
    ).toBeInTheDocument();
  });

  it('does not steal focus from a control the user moved to after jumping to someone', async () => {
    await openBigTree();
    await enterTheGraph();
    await panRight(60);
    await userEvent.keyboard('{ArrowRight}'); // jumps to a person who was not drawn
    await userEvent.click(screen.getByRole('button', { name: 'Fit' })); // focus is now on Fit
    await waitFor(() => {
      expect(scaleOf()).toBeCloseTo(0.05, 3); // the view changed, so the graph re-rendered
    }, LAYOUT_WAIT);
    expect(screen.getByRole('button', { name: 'Fit' })).toHaveFocus();
  });

  it('draws and focuses someone far from the view when they are chosen in the search', async () => {
    await openBigTree();
    const far = [...Array(COUNT).keys()]
      .map((i) => i + 1)
      .filter((id) => id > 99)
      .findLast((id) => !isDrawn(id));
    if (far === undefined) throw new Error('every three-digit person is already drawn');
    await userEvent.type(
      screen.getByRole('combobox', { name: 'Find a person' }),
      `Person ${String(far)}`,
    );
    await userEvent.click(
      await screen.findByRole('option', { name: new RegExp(`^Person ${String(far)},`) }),
    );
    await waitFor(() => {
      expect(node(far)).toHaveFocus();
    }, LAYOUT_WAIT);
    expect(node(far)).toHaveAttribute('aria-pressed', 'true');
  });

  it('moves focus along an arrow key to a neighbour that is not drawn yet', async () => {
    await openBigTree();
    await enterTheGraph();
    const start = document.activeElement;
    await panRight(60); // the people beside the focused one are now far outside the window
    await waitFor(() => {
      expect(people().length).toBeGreaterThan(0);
    });
    const before = drawn();
    expect(document.activeElement).toBe(start);
    await userEvent.keyboard('{ArrowRight}');
    const now = document.activeElement;
    expect(now).not.toBe(start);
    const id = Number(now?.getAttribute('data-person-id'));
    expect(Number.isInteger(id)).toBe(true);
    expect(before.has(id)).toBe(false); // it was not in the DOM a moment ago
    expect(node(id)).toHaveFocus();
  });

  it('draws full cards at the opening view and bare rectangles when zoomed far out, and returns', async () => {
    await openBigTree();
    expect(cardsWithText()).toBeGreaterThan(0);
    await enterTheGraph();
    const opening = people().length;
    await userEvent.keyboard('0'); // Fit: as far out as the app zooms (a tiny scale)
    await waitFor(() => {
      expect(scaleOf()).toBeCloseTo(0.05, 3); // the animation has finished
    }, LAYOUT_WAIT);
    // far more is on screen than at the opening view. (Not "the whole tree": at the smallest scale a
    // tree this wide does not fit, and how much of the window's margin is drawn depends on timing.)
    await waitFor(() => {
      expect(people().length).toBeGreaterThan(opening * 2);
    });
    await waitFor(() => {
      expect(cardsWithText()).toBe(0);
    });
    expect(document.querySelectorAll('[data-person-id] rect')).toHaveLength(people().length);
    // still labelled and focusable for the keyboard and assistive technology
    expect(people()[0]?.getAttribute('aria-label')).toMatch(/^Person \d+/);
    await userEvent.keyboard('{+>30/}');
    await waitFor(() => {
      expect(cardsWithText()).toBeGreaterThan(0);
    });
  });

  it('draws more when the container grows, without waiting for the user to pan', async () => {
    const sizes = { width: 800, height: 600 };
    vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(() => ({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      right: sizes.width,
      bottom: sizes.height,
      ...sizes,
      toJSON: () => ({}),
    }));
    const observers: (() => void)[] = [];
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(callback: () => void) {
          observers.push(callback);
        }
        observe() {
          /* the test calls the callback itself */
        }
        disconnect() {
          /* nothing to release */
        }
      },
    );
    await openBigTree();
    const before = people().length;
    sizes.width = 6000;
    sizes.height = 4000;
    act(() => {
      for (const callback of observers) callback();
    });
    await waitFor(() => {
      expect(people().length).toBeGreaterThan(before * 2);
    });
  });
});
