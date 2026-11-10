import { useEffect, useMemo, useRef, useState } from 'react';
import { describeApiError, describeWriteError } from '../api/describeError';
import { displayName } from '../domain/format';
import type { LayoutEngine } from '../layout/types';
import { useLayout } from '../layout/useLayout';
import { GraphView, type GraphViewHandle } from '../view/GraphView';
import { AddPersonPanel, type Relation } from './AddPersonPanel';
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
 * lands, which the "Laying out…" status announces. Bringing a person into view waits for that:
 * see `reveal`.
 */
export function GraphScreen({ getLayoutEngine }: Props) {
  const graph = useFamilyGraph();
  const engine = useMemo<LayoutEngine>(
    () => ({ layout: (elkGraph) => getLayoutEngine().layout(elkGraph) }),
    [getLayoutEngine],
  );
  const { layout, pending, error, retry } = useLayout(graph.data, engine);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  // The add form is open; with a relation, the new person is linked to that person once saved.
  const [adding, setAdding] = useState<{ relation?: Relation } | null>(null);
  // Something worth saying that is not tied to one form (a person added but not linked).
  const [notice, setNotice] = useState<string | null>(null);
  const viewRef = useRef<GraphViewHandle>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const addButtonRef = useRef<HTMLButtonElement>(null);
  // Someone to centre on and focus as soon as the current layout contains them (a person just
  // added is not in the layout until the new one lands).
  const revealRef = useRef<number | null>(null);
  // What is selected right now, for callbacks that finish later (a delete that was running).
  const selectedRef = useRef<number | null>(null);

  useEffect(() => {
    const id = revealRef.current;
    if (id === null || pending || !layout?.persons.some((box) => box.id === id)) return;
    revealRef.current = null;
    viewRef.current?.centerOn(id);
    viewRef.current?.focusPerson(id);
  });

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

  const selected = selectedId === null ? undefined : family.personById.get(selectedId);
  const isEmpty = family.persons.length === 0;

  /** Select someone, with no move of the view. */
  function select(id: number | null) {
    revealRef.current = null;
    selectedRef.current = id;
    setSelectedId(id);
    setNotice(null);
    if (id !== null) setAdding(null);
  }

  /** Select someone and bring them into view; focus follows, so the keyboard carries on from there. */
  function reveal(id: number) {
    select(id);
    revealRef.current = id;
  }

  return (
    <div className={styles.screen}>
      <div className={styles.bar}>
        {isEmpty ? null : (
          <SearchBox persons={family.persons} onChoose={reveal} inputRef={searchRef} />
        )}
        <button
          type="button"
          ref={addButtonRef}
          className={styles.addButton}
          onClick={() => {
            select(null);
            setAdding({});
          }}
        >
          Add person
        </button>
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
          {notice ? (
            <div role="alert" className={styles.alert}>
              <span>{notice}</span>{' '}
              <button
                type="button"
                onClick={() => {
                  setNotice(null);
                }}
              >
                Dismiss
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
        {isEmpty ? (
          <p role="status" className={styles.message}>
            Your tree is empty. Use “Add person” to start it.
          </p>
        ) : null}
        {layout && !isEmpty ? (
          <GraphView
            ref={viewRef}
            layout={layout}
            family={family}
            selectedId={selected ? selected.id : null}
            onSelect={select}
          />
        ) : null}
        {adding ? (
          <AddPersonPanel
            // a different add form (a relative's, or the plain one) must not inherit typed text or a running save
            key={
              adding.relation ? `${adding.relation.role}-${String(adding.relation.ofId)}` : 'new'
            }
            relation={adding.relation}
            onCreated={(id, outcome) => {
              if (outcome.open) {
                setAdding(null);
                reveal(id);
              }
              // said even if the user has moved on: the person exists but is not linked
              if (outcome.linkError !== undefined && outcome.relation) {
                setNotice(
                  `The new person was added, but not linked to ${outcome.relation.ofName}: ${describeWriteError(outcome.linkError)}`,
                );
              }
            }}
            onCancel={() => {
              setAdding(null);
              if (adding.relation) viewRef.current?.focusPerson(adding.relation.ofId);
              else addButtonRef.current?.focus();
            }}
          />
        ) : selected ? (
          <DetailPanel
            key={selected.id}
            family={family}
            person={selected}
            onCenter={() => {
              viewRef.current?.centerOn(selected.id);
            }}
            onClear={() => {
              // The Clear button disappears with the selection, so give focus back to the person.
              viewRef.current?.focusPerson(selected.id);
              select(null);
            }}
            onGoTo={reveal}
            onAddNew={(role, kind) => {
              setAdding({
                relation: { role, kind, ofId: selected.id, ofName: displayName(selected) },
              });
            }}
            onDeleted={(id) => {
              // the user may have chosen someone else while the delete was running
              if (selectedRef.current !== id) return;
              select(null);
              (searchRef.current ?? addButtonRef.current)?.focus();
            }}
          />
        ) : null}
      </div>
    </div>
  );
}
