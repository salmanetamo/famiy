import { useMemo, useRef, useState } from 'react';
import { describeApiError } from '../api/describeError';
import type { LayoutEngine } from '../layout/types';
import { useLayout } from '../layout/useLayout';
import { GraphView, type GraphViewHandle } from '../view/GraphView';
import { DetailPanel } from './DetailPanel';
import styles from './GraphScreen.module.css';
import { SearchBox } from './SearchBox';
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
  const viewRef = useRef<GraphViewHandle>(null);

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

  const selected = selectedId === null ? undefined : family.personById.get(selectedId);

  /** Select someone and bring them into view; focus follows, so the keyboard carries on from there. */
  function goTo(id: number) {
    setSelectedId(id);
    viewRef.current?.centerOn(id);
    viewRef.current?.focusPerson(id);
  }

  return (
    <div className={styles.screen}>
      <div className={styles.bar}>
        <SearchBox persons={family.persons} onChoose={goTo} />
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
      </div>
      <div className={styles.stage}>
        {layout ? (
          <GraphView
            ref={viewRef}
            layout={layout}
            family={family}
            selectedId={selected ? selected.id : null}
            onSelect={setSelectedId}
          />
        ) : null}
        {selected ? (
          <DetailPanel
            family={family}
            person={selected}
            onCenter={() => {
              viewRef.current?.centerOn(selected.id);
            }}
            onClear={() => {
              // The Clear button disappears with the selection, so give focus back to the person.
              viewRef.current?.focusPerson(selected.id);
              setSelectedId(null);
            }}
            onGoTo={goTo}
          />
        ) : null}
      </div>
    </div>
  );
}
