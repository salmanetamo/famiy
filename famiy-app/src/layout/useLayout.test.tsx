import type { ElkNode } from 'elkjs/lib/elk-api';
import { act, renderHook, waitFor } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { buildFamilyGraph } from '../domain/family';
import { apiGraphFrom, edgeCases, messy } from '../test/fixtures';
import { createInlineEngine } from '../test/inlineEngine';
import type { LayoutEngine } from './types';
import { useLayout } from './useLayout';

const inline = createInlineEngine();
const small = buildFamilyGraph(apiGraphFrom(edgeCases).api);
const large = buildFamilyGraph(apiGraphFrom(messy).api);

/** An engine that answers only when the test says so. */
function manualEngine() {
  const waiting: { graph: ElkNode; answer: () => void }[] = [];
  const engine: LayoutEngine = {
    layout: (graph) =>
      new Promise<ElkNode>((resolve) => {
        waiting.push({ graph, answer: () => void inline.layout(graph).then(resolve) });
      }),
  };
  return { engine, waiting };
}

describe('useLayout', () => {
  it('is pending until the layout for the current graph arrives, then settled', async () => {
    const { result } = renderHook(() => useLayout(small, inline));
    expect(result.current.pending).toBe(true);
    expect(result.current.layout).toBeNull();
    await waitFor(() => {
      expect(result.current.pending).toBe(false);
    });
    expect(result.current.layout?.persons).toHaveLength(9);
    expect(result.current.error).toBeNull();
  });

  it('keeps the previous layout on screen while a newer graph is laid out', async () => {
    const { result, rerender } = renderHook(({ family }) => useLayout(family, inline), {
      initialProps: { family: small },
    });
    await waitFor(() => {
      expect(result.current.layout?.persons).toHaveLength(9);
    });
    rerender({ family: large });
    expect(result.current.pending).toBe(true);
    expect(result.current.layout?.persons).toHaveLength(9); // still the old one
    await waitFor(() => {
      expect(result.current.layout?.persons).toHaveLength(12);
    });
    expect(result.current.pending).toBe(false);
  });

  it('ignores a late answer from an engine that has since been replaced', async () => {
    const first = manualEngine();
    const second = manualEngine();
    const { result, rerender } = renderHook(({ engine }) => useLayout(small, engine), {
      initialProps: { engine: first.engine },
    });
    rerender({ engine: second.engine });
    await waitFor(() => {
      expect(second.waiting).toHaveLength(1);
    });
    act(() => {
      second.waiting[0]?.answer();
    });
    await waitFor(() => {
      expect(result.current.pending).toBe(false);
    });
    const settled = result.current.layout;

    // the replaced engine answers late: it must not overwrite the current layout or the status
    await act(async () => {
      first.waiting[0]?.answer();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });
    expect(result.current.layout).toBe(settled);
    expect(result.current.error).toBeNull();
  });

  it('reports an engine failure and keeps the last good layout', async () => {
    const { result, rerender } = renderHook(
      ({ engine, family }: { engine: LayoutEngine; family: typeof small }) =>
        useLayout(family, engine),
      { initialProps: { engine: inline, family: small } },
    );
    await waitFor(() => {
      expect(result.current.layout?.persons).toHaveLength(9);
    });
    const broken: LayoutEngine = { layout: () => Promise.reject(new Error('worker crashed')) };
    rerender({ engine: broken, family: large });
    await waitFor(() => {
      expect(result.current.error?.message).toBe('worker crashed');
    });
    expect(result.current.layout?.persons).toHaveLength(9);
    expect(result.current.pending).toBe(false);
  });

  it('does not carry an old failure over to the next graph', async () => {
    const broken: LayoutEngine = { layout: () => Promise.reject(new Error('worker crashed')) };
    const { result, rerender } = renderHook(
      ({ engine, family }: { engine: LayoutEngine; family: typeof small }) =>
        useLayout(family, engine),
      { initialProps: { engine: broken, family: small } },
    );
    await waitFor(() => {
      expect(result.current.error?.message).toBe('worker crashed');
    });
    rerender({ engine: inline, family: large });
    expect(result.current.pending).toBe(true);
    expect(result.current.error).toBeNull(); // the failure was about the previous graph
    await waitFor(() => {
      expect(result.current.layout?.persons).toHaveLength(12);
    });
  });

  it('retry lays the same graph out again and clears the failure while it runs', async () => {
    let failing = true;
    const flaky: LayoutEngine = {
      layout: (graph) =>
        failing ? Promise.reject(new Error('worker crashed')) : inline.layout(graph),
    };
    const { result } = renderHook(() => useLayout(small, flaky));
    await waitFor(() => {
      expect(result.current.error).not.toBeNull();
    });
    failing = false;
    act(() => {
      result.current.retry();
    });
    expect(result.current.error).toBeNull();
    expect(result.current.pending).toBe(true);
    await waitFor(() => {
      expect(result.current.layout?.persons).toHaveLength(9);
    });
    expect(result.current.pending).toBe(false);
  });

  it('does nothing without a graph', () => {
    const { result } = renderHook(() => useLayout(undefined, inline));
    expect(result.current).toMatchObject({ layout: null, pending: false, error: null });
  });
});
