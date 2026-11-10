import { useMemo, useState } from 'react';
import { describeApiError } from '../api/describeError';
import type { LayoutEngine } from '../layout/types';
import { useLayout } from '../layout/useLayout';
import { GraphView } from '../view/GraphView';
import styles from './GraphScreen.module.css';
import { useFamilyGraph } from './useFamilyGraph';

interface Props {
  /** Resolved the first time a layout is needed, so nothing starts a worker for an empty account. */
  getLayoutEngine: () => LayoutEngine;
}

/**
 * Fetches the account's graph, lays it out and shows it. Selection is view state, kept here.
 *
 * While a newer graph is being laid out the previous layout stays on screen (so the view does not
 * flash), drawn with the new people's labels; relatives added since then appear when the layout
 * lands, which the "Laying out…" status announces.
 */
export function GraphScreen({ getLayoutEngine }: Props) {
  const graph = useFamilyGraph();
  const engine = useMemo<LayoutEngine>(
    () => ({ layout: (elkGraph) => getLayoutEngine().layout(elkGraph) }),
    [getLayoutEngine],
  );
  const { layout, pending, error, retry } = useLayout(graph.data, engine);
  const [selectedId, setSelectedId] = useState<number | null>(null);

  const family = graph.data;
  if (family === undefined) {
    if (graph.isPending) return <p role="status">Loading your family…</p>;
    return (
      <div className={styles.message}>
        <p role="alert">{describeApiError(graph.error)}</p>
        <button
          type="button"
          onClick={() => {
            void graph.refetch();
          }}
        >
          Try again
        </button>
      </div>
    );
  }
  if (family.persons.length === 0) {
    return (
      <p role="status" className={styles.message}>
        Your tree is empty. People will appear here once they have been added.
      </p>
    );
  }

  return (
    <div className={styles.screen}>
      <div className={styles.status}>
        <div role="status">{pending ? 'Laying out your family…' : ''}</div>
        {graph.isError ? (
          // A failed background refresh must not replace the graph the user is looking at.
          <div role="alert" className={styles.alert}>
            <span>Could not refresh your family: {describeApiError(graph.error)}</span>{' '}
            <button
              type="button"
              onClick={() => {
                void graph.refetch();
              }}
            >
              Try again
            </button>
          </div>
        ) : null}
        {error ? (
          <div role="alert" className={styles.alert}>
            <span>Could not lay out your family: {error.message}</span>{' '}
            <button type="button" onClick={retry}>
              Try again
            </button>
          </div>
        ) : null}
      </div>
      {layout ? (
        <GraphView
          layout={layout}
          family={family}
          selectedId={selectedId !== null && family.personById.has(selectedId) ? selectedId : null}
          onSelect={setSelectedId}
        />
      ) : null}
    </div>
  );
}
