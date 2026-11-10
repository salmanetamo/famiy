import type { ElkNode } from 'elkjs/lib/elk-api';
import { fireEvent, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { LayoutEngine } from '../layout/types';
import { errorResponse, jsonResponse, renderWithClient, stubFetch } from '../test-utils';
import { apiGraphFrom, edgeCases, emptyGraph, messy } from '../test/fixtures';
import { createInlineEngine } from '../test/inlineEngine';
import { GraphScreen } from './GraphScreen';

afterEach(() => {
  vi.unstubAllGlobals();
});

const engine = createInlineEngine();
const getEngine = () => engine;

function renderScreen(family = messy, getLayoutEngine: () => LayoutEngine = getEngine) {
  const { api, idOf } = apiGraphFrom(family);
  stubFetch(() => jsonResponse(200, api));
  renderWithClient(<GraphScreen getLayoutEngine={getLayoutEngine} />);
  return { idOf };
}

const people = () => [...document.querySelectorAll<SVGGElement>('[data-person-id]')];
const node = (id: number) => {
  const found = document.querySelector<SVGGElement>(`[data-person-id="${String(id)}"]`);
  if (!found) throw new Error(`no node for ${String(id)}`);
  return found;
};
const svgRoot = (): SVGSVGElement => {
  const svg = document.querySelector('svg');
  if (!svg) throw new Error('no svg');
  return svg;
};
const world = () => {
  const group = document.querySelector<SVGGElement>('svg g');
  if (!group) throw new Error('no world group');
  return group;
};
// jsdom rewrites numbers in style strings (0.77 becomes 7.7e-1), so read real numbers, not digits
const numbersIn = (text: string): number[] =>
  (text.match(/-?\d+(?:\.\d+)?(?:e-?\d+)?/g) ?? []).map(Number);
const scaleOf = (): number =>
  numbersIn(/scale\(([^)]*)\)/.exec(world().style.transform)?.[1] ?? '')[0] ?? NaN;
const translateOf = (text: string): [number, number] => {
  const [x, y] = numbersIn(/translate\(([^)]*)\)/.exec(text)?.[1] ?? '');
  return [x ?? NaN, y ?? NaN];
};

async function waitForPeople(count: number) {
  await waitFor(() => {
    expect(people()).toHaveLength(count);
  });
}

describe('GraphScreen states', () => {
  it('shows loading, then the people', async () => {
    renderScreen();
    expect(screen.getByRole('status')).toHaveTextContent('Loading your family');
    await waitForPeople(12);
  });

  it('says so when the account has no people, without starting a layout engine', async () => {
    const getLayoutEngine = vi.fn(getEngine);
    stubFetch(() => jsonResponse(200, emptyGraph()));
    renderWithClient(<GraphScreen getLayoutEngine={getLayoutEngine} />);
    expect(await screen.findByText(/your tree is empty/i)).toBeInTheDocument();
    expect(getLayoutEngine).not.toHaveBeenCalled();
  });

  it('explains a failed load and retries', async () => {
    let healthy = false;
    stubFetch(() =>
      healthy
        ? jsonResponse(200, apiGraphFrom(edgeCases).api)
        : errorResponse(500, 'internal', 'boom'),
    );
    renderWithClient(<GraphScreen getLayoutEngine={getEngine} />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Something went wrong (internal)');
    healthy = true;
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    await waitForPeople(9);
  });

  it('tells the user when the server is not reachable', async () => {
    stubFetch(() => new Response('Bad Gateway', { status: 502 }));
    renderWithClient(<GraphScreen getLayoutEngine={getEngine} />);
    expect(await screen.findByRole('alert')).toHaveTextContent(/the server isn't reachable/i);
  });

  it('shows a layout-in-progress message until the layout arrives', async () => {
    let finish: (graph: ElkNode) => void = () => undefined;
    const slow: LayoutEngine = {
      layout: (graph) =>
        new Promise<ElkNode>((resolve) => {
          finish = (laid) => {
            resolve(laid);
          };
          void engine.layout(graph).then(finish);
        }),
    };
    renderScreen(messy, () => slow);
    expect(await screen.findByText('Laying out your family…')).toBeInTheDocument();
    await waitForPeople(12);
    expect(screen.queryByText('Laying out your family…')).not.toBeInTheDocument();
  });

  it('reports a layout failure instead of staying blank', async () => {
    const broken: LayoutEngine = { layout: () => Promise.reject(new Error('worker crashed')) };
    renderScreen(messy, () => broken);
    expect(
      await screen.findByText(/could not lay out your family: worker crashed/i),
    ).toBeInTheDocument();
  });
});

describe('GraphView', () => {
  it('draws every person as a labelled button with name and life span, and edges between them', async () => {
    renderScreen();
    await waitForPeople(12);
    expect(screen.getByRole('button', { name: 'Abou Diallo, c. 1930' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Rama, c. 1960' })).toBeInTheDocument();
    expect(document.querySelectorAll('polyline').length).toBeGreaterThan(10);
    expect(node(1).textContent).toContain('Abou Diallo');
  });

  it('truncates a very long name on the card but keeps the full name in the label', async () => {
    renderScreen(edgeCases);
    await waitForPeople(9);
    const uma = screen.getByRole('button', { name: /Uma Alexandra-Maria Whitfield-Montgomery/ });
    expect(uma.querySelector('text')?.textContent).toMatch(/…$/);
    expect(uma.querySelector('title')?.textContent).toMatch(/Whitfield-Montgomery/);
  });

  it('is a single tab stop, on the eldest person to begin with', async () => {
    renderScreen();
    await waitForPeople(12);
    const stops = people().filter((p) => p.tabIndex === 0);
    expect(stops).toHaveLength(1);
    expect(stops[0]?.getAttribute('aria-label')).toMatch(/^Abou Diallo/);
  });

  it('selects on click, shows a summary, and clears with the Clear button or a click on the background', async () => {
    const { idOf } = renderScreen();
    await waitForPeople(12);
    const ali = node(idOf.c1 ?? -1);
    await userEvent.click(ali);
    expect(ali).toHaveAttribute('aria-pressed', 'true');
    expect(ali).toHaveAccessibleName('Ali Diallo, c. 1982');
    expect(screen.getByText('Ali Diallo', { selector: 'strong' })).toBeInTheDocument(); // the summary
    expect(people().filter((p) => p.getAttribute('aria-pressed') === 'true')).toHaveLength(1);

    await userEvent.click(screen.getByRole('button', { name: 'Clear' }));
    expect(ali).toHaveAttribute('aria-pressed', 'false');

    await userEvent.click(ali);
    fireEvent.click(svgRoot());
    expect(ali).toHaveAttribute('aria-pressed', 'false');
  });

  it('moves the single tab stop to the selected person', async () => {
    const { idOf } = renderScreen();
    await waitForPeople(12);
    await userEvent.click(node(idOf.c2 ?? -1));
    const stops = people().filter((p) => p.tabIndex === 0);
    expect(stops.map((p) => p.dataset.personId)).toEqual([String(idOf.c2)]);
  });

  it('moves between relatives with the arrow keys and selects with Enter', async () => {
    const { idOf } = renderScreen();
    await waitForPeople(12);
    await userEvent.tab(); // into the graph: Abou, the eldest
    expect(node(idOf.g1 ?? -1)).toHaveFocus();
    await userEvent.keyboard('{ArrowDown}'); // eldest child: Idi (1958) before Nana (1961)
    expect(node(idOf.p1 ?? -1)).toHaveFocus();
    await userEvent.keyboard('{ArrowDown}'); // Idi's eldest child: Ali (1982)
    expect(node(idOf.c1 ?? -1)).toHaveFocus();
    await userEvent.keyboard('{ArrowUp}'); // back to a parent of Ali: Idi (1958) before Rama (1960)
    expect(node(idOf.p1 ?? -1)).toHaveFocus();
    await userEvent.keyboard('{Enter}');
    expect(node(idOf.p1 ?? -1)).toHaveAttribute('aria-pressed', 'true');
    await userEvent.keyboard('{Escape}');
    expect(node(idOf.p1 ?? -1)).toHaveAttribute('aria-pressed', 'false');
  });

  it('stays put when there is nowhere to go, and keeps one tab stop after moving', async () => {
    const { idOf } = renderScreen();
    await waitForPeople(12);
    await userEvent.tab();
    await userEvent.keyboard('{ArrowUp}'); // a founder has no parent
    expect(node(idOf.g1 ?? -1)).toHaveFocus();
    await userEvent.keyboard('{ArrowDown}');
    expect(people().filter((p) => p.tabIndex === 0)).toHaveLength(1);
  });

  it('does not move focus on Shift+arrow, which pans the view instead', async () => {
    const { idOf } = renderScreen();
    await waitForPeople(12);
    await userEvent.tab();
    const before = world().style.transform;
    await userEvent.keyboard('{Shift>}{ArrowRight}{/Shift}');
    expect(node(idOf.g1 ?? -1)).toHaveFocus();
    expect(world().style.transform).not.toBe(before);
  });

  it('starts with the whole family in view, and + / - / 0 change and reset the zoom', async () => {
    renderScreen();
    await waitForPeople(12);
    const fitted = scaleOf();
    expect(fitted).toBeGreaterThan(0.2);
    await userEvent.tab();
    await userEvent.keyboard('+');
    expect(scaleOf()).toBeCloseTo(fitted * 1.25, 5);
    await userEvent.keyboard('-');
    expect(scaleOf()).toBeCloseTo(fitted, 5);
    await userEvent.keyboard('+');
    await userEvent.keyboard('0');
    await waitFor(() => {
      expect(scaleOf()).toBeCloseTo(fitted, 3);
    });
  });

  it('zooms with the mouse wheel and stops the page from scrolling', async () => {
    renderScreen();
    await waitForPeople(12);
    const before = scaleOf();
    const viewport = svgRoot().parentElement;
    if (!viewport) throw new Error('the graph has no viewport element');
    const zoomIn = new WheelEvent('wheel', { deltaY: -300, bubbles: true, cancelable: true });
    viewport.dispatchEvent(zoomIn);
    expect(scaleOf()).toBeGreaterThan(before);
    expect(zoomIn.defaultPrevented).toBe(true);
    const zoomOut = new WheelEvent('wheel', { deltaY: 600, bubbles: true, cancelable: true });
    viewport.dispatchEvent(zoomOut);
    expect(scaleOf()).toBeLessThan(before * 1.6);
  });

  it('centers on the selected person with the Center button', async () => {
    const { idOf } = renderScreen();
    await waitForPeople(12);
    const umar = node(idOf.d1 ?? -1);
    await userEvent.click(umar);
    await userEvent.click(screen.getByRole('button', { name: 'Center' }));
    await waitFor(() => {
      const [x, y] = translateOf(umar.getAttribute('transform') ?? '');
      const [tx, ty] = translateOf(world().style.transform);
      const k = scaleOf();
      // the card's centre (80, 28 inside the card) lands in the middle of the fallback 800x600 view
      expect(x * k + 80 * k + tx).toBeCloseTo(400, 0);
      expect(y * k + 28 * k + ty).toBeCloseTo(300, 0);
    });
  });

  it('centers on a person when they are double-clicked', async () => {
    const { idOf } = renderScreen();
    await waitForPeople(12);
    const hawa = node(idOf.c2 ?? -1);
    await userEvent.dblClick(hawa);
    expect(hawa).toHaveAttribute('aria-pressed', 'true');
  });
});

describe('GraphView controls sit on top of the graph without acting as the background', () => {
  it.each(['Center', 'Zoom in', 'Zoom out', 'Fit'])(
    'pressing %s keeps the selection',
    async (name) => {
      const { idOf } = renderScreen();
      await waitForPeople(12);
      const ali = node(idOf.c1 ?? -1);
      await userEvent.click(ali);
      await userEvent.click(screen.getByRole('button', { name }));
      expect(ali).toHaveAttribute('aria-pressed', 'true');
      expect(screen.getByRole('button', { name: 'Clear' })).toBeInTheDocument();
    },
  );

  it('still clears the selection when the empty background is clicked', async () => {
    const { idOf } = renderScreen();
    await waitForPeople(12);
    const ali = node(idOf.c1 ?? -1);
    await userEvent.click(ali);
    expect(ali).toHaveAttribute('aria-pressed', 'true');
    await userEvent.click(svgRoot());
    expect(ali).toHaveAttribute('aria-pressed', 'false');
  });
});

describe('focus after the selection summary goes away', () => {
  it('returns focus to the person when Clear removes the button that had it', async () => {
    const { idOf } = renderScreen();
    await waitForPeople(12);
    const ali = node(idOf.c1 ?? -1);
    await userEvent.click(ali);
    await userEvent.click(screen.getByRole('button', { name: 'Clear' }));
    expect(screen.queryByRole('button', { name: 'Clear' })).not.toBeInTheDocument();
    expect(ali).toHaveFocus();
  });
});

describe('dragging the graph', () => {
  // jsdom has no pointer capture; the app only calls it once a press has moved far enough to be a drag
  const capture = vi.fn();
  const originalCapture = Object.getOwnPropertyDescriptor(Element.prototype, 'setPointerCapture');
  beforeEach(() => {
    capture.mockClear();
    Object.defineProperty(Element.prototype, 'setPointerCapture', {
      value: capture,
      configurable: true,
      writable: true,
    });
  });
  afterEach(() => {
    if (originalCapture)
      Object.defineProperty(Element.prototype, 'setPointerCapture', originalCapture);
    else Reflect.deleteProperty(Element.prototype, 'setPointerCapture');
  });

  const press = (target: Element, x: number, y: number) =>
    fireEvent.pointerDown(target, { button: 0, pointerId: 1, clientX: x, clientY: y });
  const move = (target: Element, x: number, y: number) =>
    fireEvent.pointerMove(target, { pointerId: 1, clientX: x, clientY: y });
  const release = (target: Element, x: number, y: number) =>
    fireEvent.pointerUp(target, { pointerId: 1, clientX: x, clientY: y });

  it('pans by the distance the pointer moved, after passing a small threshold', async () => {
    renderScreen();
    await waitForPeople(12);
    const [x0, y0] = translateOf(world().style.transform);
    press(svgRoot(), 100, 100);
    move(svgRoot(), 102, 101); // under the threshold: not a drag yet
    expect(translateOf(world().style.transform)).toEqual([x0, y0]);
    move(svgRoot(), 160, 140);
    const [x1, y1] = translateOf(world().style.transform);
    expect(x1 - x0).toBeCloseTo(60);
    expect(y1 - y0).toBeCloseTo(40);
    expect(capture).toHaveBeenCalledTimes(1);
    release(svgRoot(), 160, 140);
  });

  it('does not pan when the press starts on a control such as Zoom in', async () => {
    renderScreen();
    await waitForPeople(12);
    const before = world().style.transform;
    const zoomIn = screen.getByRole('button', { name: 'Zoom in' });
    press(zoomIn, 100, 100);
    move(zoomIn, 200, 200);
    expect(world().style.transform).toBe(before);
    expect(capture).not.toHaveBeenCalled();
  });

  it('does not treat the click after a drag as a click on the background or a person', async () => {
    const { idOf } = renderScreen();
    await waitForPeople(12);
    const ali = node(idOf.c1 ?? -1);
    await userEvent.click(ali);
    press(svgRoot(), 100, 100);
    move(svgRoot(), 180, 100);
    release(svgRoot(), 180, 100);
    fireEvent.click(svgRoot()); // the browser sends a click after the pointer is released
    expect(ali).toHaveAttribute('aria-pressed', 'true');
  });
});

describe('review findings (slice 2)', () => {
  it('arrow keys still move focus after Clear returned focus to a different person', async () => {
    const { idOf } = renderScreen();
    await waitForPeople(12);
    await userEvent.click(node(idOf.p1 ?? -1)); // select Idi
    await userEvent.keyboard('{ArrowDown}'); // focus moves to Ali, Idi's eldest child
    expect(node(idOf.c1 ?? -1)).toHaveFocus();
    await userEvent.click(screen.getByRole('button', { name: 'Clear' })); // focus returns to Idi
    expect(node(idOf.p1 ?? -1)).toHaveFocus();
    await userEvent.keyboard('{ArrowDown}');
    expect(node(idOf.c1 ?? -1)).toHaveFocus();
  });

  it('a person is a toggle: the label does not repeat "selected", and pressing again deselects', async () => {
    const { idOf } = renderScreen();
    await waitForPeople(12);
    const ali = node(idOf.c1 ?? -1);
    await userEvent.click(ali);
    expect(ali).toHaveAttribute('aria-pressed', 'true');
    expect(ali).toHaveAccessibleName('Ali Diallo, c. 1982');
    await userEvent.click(ali);
    expect(ali).toHaveAttribute('aria-pressed', 'false');
    ali.focus();
    await userEvent.keyboard('{Enter}');
    expect(ali).toHaveAttribute('aria-pressed', 'true');
    await userEvent.keyboard('{Enter}');
    expect(ali).toHaveAttribute('aria-pressed', 'false');
  });

  it('keeps showing the graph when a background refresh fails, with a non-blocking message', async () => {
    const { api } = apiGraphFrom(messy);
    let failing = false;
    stubFetch(() => (failing ? errorResponse(500, 'internal', 'boom') : jsonResponse(200, api)));
    const { client } = renderWithClient(<GraphScreen getLayoutEngine={getEngine} />);
    await waitForPeople(12);
    failing = true;
    await client.refetchQueries({ queryKey: ['graph'] });
    expect(await screen.findByText(/could not refresh/i)).toBeInTheDocument();
    expect(people()).toHaveLength(12);
  });

  it('announces a layout failure as an alert and offers to try again', async () => {
    let broken = true;
    const engineThatRecovers: LayoutEngine = {
      layout: (graph) =>
        broken ? Promise.reject(new Error('worker crashed')) : engine.layout(graph),
    };
    renderScreen(messy, () => engineThatRecovers);
    expect(await screen.findByRole('alert')).toHaveTextContent(/worker crashed/i);
    broken = false;
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    await waitForPeople(12);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('zooms by a similar amount for a line-mode wheel (Firefox) as for a pixel-mode wheel', async () => {
    renderScreen();
    await waitForPeople(12);
    const viewport = svgRoot().parentElement;
    if (!viewport) throw new Error('no viewport');
    const before = scaleOf();
    viewport.dispatchEvent(new WheelEvent('wheel', { deltaY: -3, deltaMode: 1, cancelable: true }));
    const afterLines = scaleOf();
    expect(afterLines / before).toBeGreaterThan(1.05); // 3 lines is a clear step, not a 0.5% nudge
  });
});

describe('a drag that lost its pointer', () => {
  const capture = vi.fn();
  beforeEach(() => {
    Object.defineProperty(Element.prototype, 'setPointerCapture', {
      value: capture,
      configurable: true,
      writable: true,
    });
  });
  afterEach(() => {
    Reflect.deleteProperty(Element.prototype, 'setPointerCapture');
  });

  it('does not pan when the pointer returns with no button held (released outside the window)', async () => {
    renderScreen();
    await waitForPeople(12);
    const before = world().style.transform;
    fireEvent.pointerDown(svgRoot(), {
      button: 0,
      buttons: 1,
      pointerId: 1,
      pointerType: 'mouse',
      clientX: 100,
      clientY: 100,
    });
    // the button is released outside the browser window: no pointerup reaches the page
    fireEvent.pointerMove(svgRoot(), {
      buttons: 0,
      pointerId: 1,
      pointerType: 'mouse',
      clientX: 220,
      clientY: 200,
    });
    expect(world().style.transform).toBe(before);
  });

  it('stops the drag when pointer capture is lost', async () => {
    renderScreen();
    await waitForPeople(12);
    fireEvent.pointerDown(svgRoot(), {
      button: 0,
      buttons: 1,
      pointerId: 1,
      pointerType: 'mouse',
      clientX: 100,
      clientY: 100,
    });
    fireEvent.pointerMove(svgRoot(), {
      buttons: 1,
      pointerId: 1,
      pointerType: 'mouse',
      clientX: 160,
      clientY: 100,
    });
    const moved = world().style.transform;
    fireEvent.lostPointerCapture(svgRoot(), { pointerId: 1 });
    fireEvent.pointerMove(svgRoot(), {
      buttons: 1,
      pointerId: 1,
      pointerType: 'mouse',
      clientX: 300,
      clientY: 100,
    });
    expect(world().style.transform).toBe(moved);
  });
});

describe('the tab stop follows where focus really is', () => {
  it('moves to the person an arrow key just focused, and the old one stops being a tab stop', async () => {
    const { idOf } = renderScreen();
    await waitForPeople(12);
    await userEvent.tab(); // Abou, the eldest
    const abou = node(idOf.g1 ?? -1);
    expect(abou).toHaveFocus();
    await userEvent.keyboard('{ArrowDown}'); // Idi
    const idi = node(idOf.p1 ?? -1);
    expect(idi).toHaveFocus();
    expect(idi.tabIndex).toBe(0);
    expect(abou.tabIndex).toBe(-1);
    expect(people().filter((p) => p.tabIndex === 0)).toHaveLength(1);
  });
});
