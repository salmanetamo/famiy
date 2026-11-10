import { useCallback, useEffect, useMemo, useState } from 'react';
import type { FamilyGraph } from '../domain/family';
import { LayoutRunner } from './layoutRunner';
import type { Layout, LayoutEngine } from './types';

export interface LayoutState {
  /** The most recently finished layout (kept on screen while a newer one is computed). */
  layout: Layout | null;
  /** True while the current graph has no finished layout yet. */
  pending: boolean;
  error: Error | null;
  /** Lays the current graph out again after a failure. */
  retry: () => void;
}

interface Settled {
  layout: Layout | null;
  error: Error | null;
  family: FamilyGraph | undefined;
}

/** Lays out `family` in the engine whenever it changes; only the latest request's result is kept. */
export function useLayout(family: FamilyGraph | undefined, engine: LayoutEngine): LayoutState {
  const runner = useMemo(() => new LayoutRunner(engine), [engine]);
  const [settled, setSettled] = useState<Settled>({ layout: null, error: null, family: undefined });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    if (!family) return;
    let active = true;
    runner.request(family).then(
      (layout) => {
        if (active && layout) setSettled({ layout, error: null, family });
      },
      (error: unknown) => {
        if (active) {
          setSettled((previous) => ({
            layout: previous.layout,
            error: error instanceof Error ? error : new Error(String(error)),
            family,
          }));
        }
      },
    );
    return () => {
      active = false;
    };
  }, [family, runner, attempt]);

  const retry = useCallback(() => {
    setSettled((previous) => ({ ...previous, error: null, family: undefined }));
    setAttempt((count) => count + 1);
  }, []);

  return {
    layout: settled.layout,
    pending: family !== undefined && settled.family !== family,
    // An error belongs to the graph it happened for, not to whatever graph comes next.
    error: settled.family === family ? settled.error : null,
    retry,
  };
}
