import { useEffect, useId, useRef, useState } from 'react';
import { describeApiError, describeWriteError } from '../api/describeError';
import type { FamilyGraph, Person } from '../domain/family';
import type { PartnerKind } from '../api/types';
import { displayName, formatDate, lifespan } from '../domain/format';
import { formFromPerson, toPatchBody, type PersonFormValues } from '../domain/personForm';
import styles from './DetailPanel.module.css';
import { PersonForm } from './PersonForm';
import { Relationships, type Role } from './Relationships';
import { useDeletePerson, useUpdatePerson } from './usePersonMutations';
import { usePersonDetail } from './usePersonDetail';

interface Props {
  family: FamilyGraph;
  person: Person;
  onCenter: () => void;
  onClear: () => void;
  /** Select another person (a relative) and bring them into view. */
  onGoTo: (id: number) => void;
  /** The person was deleted and the graph no longer has them. */
  onDeleted: (id: number) => void;
  /** Start adding a new person who is then linked to this one in `role`. */
  onAddNew: (role: Role, kind: PartnerKind) => void;
}

type Mode = 'view' | 'edit' | 'confirmDelete';

/**
 * Everything known about the selected person, and where they are edited or deleted. The summary and
 * relatives come from the graph that is already loaded, so the panel is useful immediately; only
 * the bio and residence are fetched. Bio is free text: it is shown as plain text, never as markup.
 *
 * Mount it with `key={person.id}` so one person's form or confirmation never carries over to another.
 */
export function DetailPanel({
  family,
  person,
  onCenter,
  onClear,
  onGoTo,
  onDeleted,
  onAddNew,
}: Props) {
  const titleId = useId();
  const detail = usePersonDetail(person.id);
  const update = useUpdatePerson(person.id);
  const remove = useDeletePerson();
  const [mode, setMode] = useState<Mode>('view');
  // What the form was opened with. Patches are computed against this, not against live query
  // data, which a refetch can replace while the form is open.
  const [baseline, setBaseline] = useState<PersonFormValues | null>(null);
  const titleRef = useRef<HTMLHeadingElement>(null);
  const editRef = useRef<HTMLButtonElement>(null);
  const deleteRef = useRef<HTMLButtonElement>(null);
  const cancelDeleteRef = useRef<HTMLButtonElement>(null);
  // Buttons that disappear while a form or confirmation is open get focus back when it closes.
  const restoreFocus = useRef<'edit' | 'delete' | null>(null);

  useEffect(() => {
    const target = restoreFocus.current;
    if (target === null || mode !== 'view') return;
    restoreFocus.current = null;
    (target === 'edit' ? editRef : deleteRef).current?.focus();
  });
  useEffect(() => {
    if (mode === 'confirmDelete') cancelDeleteRef.current?.focus();
  }, [mode]);

  const span = lifespan(person);
  const editing = mode === 'edit' ? baseline : null;
  const busy = update.isPending || remove.isPending;

  function save(original: PersonFormValues, values: PersonFormValues) {
    const patch = toPatchBody(values, original);
    if (Object.keys(patch).length === 0) {
      restoreFocus.current = 'edit';
      setMode('view'); // nothing changed, so there is nothing to send
      return;
    }
    update.mutateAsync(patch).then(
      () => {
        restoreFocus.current = 'edit';
        setMode('view');
      },
      () => undefined, // shown by the form through `update.error`
    );
  }

  function confirmDelete() {
    if (busy) return;
    remove.mutateAsync(person.id).then(
      () => {
        onDeleted(person.id);
      },
      () => undefined, // shown in the confirmation
    );
  }

  return (
    <section className={styles.panel} aria-labelledby={titleId}>
      <h2 id={titleId} ref={titleRef} tabIndex={-1} className={styles.title}>
        {displayName(person)}
      </h2>
      {span ? <p className={styles.span}>{span}</p> : null}

      {editing ? (
        <PersonForm
          initial={editing}
          submitLabel="Save"
          pending={update.isPending}
          error={update.error}
          hasError={update.isError}
          onSubmit={(values) => {
            save(editing, values);
          }}
          onCancel={() => {
            update.reset();
            restoreFocus.current = 'edit';
            setMode('view');
          }}
        />
      ) : (
        <>
          <div className={styles.actions}>
            <button type="button" onClick={onCenter}>
              Center
            </button>{' '}
            <button type="button" onClick={onClear}>
              Clear
            </button>{' '}
            <button
              type="button"
              ref={editRef}
              // the form needs the notes and residence, which arrive with the detail request
              disabled={!detail.data || mode === 'confirmDelete'}
              onClick={() => {
                if (detail.data) setBaseline(formFromPerson(detail.data));
                setMode('edit');
              }}
            >
              Edit
            </button>{' '}
            <button
              type="button"
              ref={deleteRef}
              disabled={mode === 'confirmDelete'}
              onClick={() => {
                remove.reset();
                setMode('confirmDelete');
              }}
            >
              Delete
            </button>
          </div>

          {mode === 'confirmDelete' ? (
            <div
              role="alertdialog"
              aria-labelledby={`${titleId}-confirm`}
              className={styles.confirm}
              onKeyDown={(event) => {
                if (event.key === 'Escape' && !busy) {
                  restoreFocus.current = 'delete';
                  setMode('view');
                }
              }}
            >
              <p id={`${titleId}-confirm`}>
                Delete {displayName(person)} permanently? Their links to parents, children and
                partners are removed too. Those people stay in the tree. This cannot be undone.
              </p>
              {remove.isError ? (
                <p role="alert" className={styles.confirmError}>
                  {describeWriteError(remove.error)}
                </p>
              ) : null}
              <button type="button" onClick={confirmDelete} aria-disabled={busy}>
                {remove.isPending ? 'Deleting…' : 'Delete permanently'}
              </button>{' '}
              <button
                type="button"
                ref={cancelDeleteRef}
                disabled={busy}
                onClick={() => {
                  restoreFocus.current = 'delete';
                  setMode('view');
                }}
              >
                Cancel
              </button>
            </div>
          ) : null}

          <dl className={styles.facts}>
            {person.birthDate !== null ? (
              <>
                <dt>Born</dt>
                <dd>{formatDate(person.birthDate, person.birthApprox)}</dd>
              </>
            ) : null}
            {person.deathDate !== null ? (
              <>
                <dt>Died</dt>
                <dd>{formatDate(person.deathDate)}</dd>
              </>
            ) : null}
            {detail.data?.residence ? (
              <>
                <dt>Residence</dt>
                <dd>{detail.data.residence}</dd>
              </>
            ) : null}
            {detail.data?.bio ? (
              <>
                <dt>Notes</dt>
                <dd className={styles.bio}>{detail.data.bio}</dd>
              </>
            ) : null}
          </dl>

          <Relationships
            family={family}
            person={person}
            onGoTo={onGoTo}
            onAddNew={onAddNew}
            // the buttons that were used may be gone after a write, so focus moves to the person's name
            onWritten={() => {
              titleRef.current?.focus();
            }}
          />
          {detail.isPending ? <p className={styles.muted}>Loading more details…</p> : null}
          {detail.isError ? (
            // The summary above is still valid, so this does not replace the panel.
            <div role="alert" className={styles.error}>
              <span>Could not load more details: {describeApiError(detail.error)}</span>{' '}
              <button
                type="button"
                onClick={() => {
                  void detail.refetch();
                }}
              >
                Try again
              </button>
            </div>
          ) : null}
        </>
      )}
    </section>
  );
}
