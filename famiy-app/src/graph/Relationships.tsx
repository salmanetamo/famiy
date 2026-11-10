import { useEffect, useId, useMemo, useRef, useState } from 'react';
import { describeWriteError } from '../api/describeError';
import type { PartnerKind } from '../api/types';
import {
  childCandidates,
  MAX_PARENTS,
  parentCandidates,
  partnerCandidates,
} from '../domain/candidates';
import { relativesOf, type FamilyGraph, type Person } from '../domain/family';
import { displayName, personLabel } from '../domain/format';
import styles from './Relationships.module.css';
import { SearchBox } from './SearchBox';
import {
  useAddParent,
  useAddPartner,
  useRemoveParent,
  useRemovePartner,
  useUpdatePartner,
} from './useRelationshipMutations';

export type Role = 'parent' | 'partner' | 'child';

const KINDS: readonly { value: PartnerKind; label: string }[] = [
  { value: 'marriage', label: 'Marriage' },
  { value: 'partnership', label: 'Partnership' },
  { value: 'other', label: 'Other' },
];
const isKind = (value: string): value is PartnerKind => KINDS.some((kind) => kind.value === value);

const ROLE_TEXT: Record<Role, { add: string; find: string }> = {
  parent: { add: 'Add parent', find: 'Find a parent' },
  partner: { add: 'Add partner', find: 'Find a partner' },
  child: { add: 'Add child', find: 'Find a child' },
};

interface Props {
  family: FamilyGraph;
  person: Person;
  onGoTo: (id: number) => void;
  /** Start adding a brand-new person who is then linked in this role. */
  onAddNew: (role: Role, kind: PartnerKind) => void;
  /** Called after a write succeeded; the buttons that were used may be gone, so focus moves on. */
  onWritten: () => void;
}

interface PickerProps {
  role: Role;
  personName: string;
  candidates: readonly Person[];
  kind: PartnerKind;
  onKind: (kind: PartnerKind) => void;
  busy: boolean;
  onChoose: (id: number) => void;
  onAddNew: () => void;
  onCancel: () => void;
}

/** Choose who to link: a search over the people who may validly be chosen, or a new person. */
function Picker({
  role,
  personName,
  candidates,
  kind,
  onKind,
  busy,
  onChoose,
  onAddNew,
  onCancel,
}: PickerProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const cancelRef = useRef<HTMLButtonElement>(null);
  const kindId = useId();
  const hasCandidates = candidates.length > 0;
  // aria-disabled does not stop a click or a key, so each action checks for itself
  const cancel = () => {
    if (!busy) onCancel();
  };
  const addNew = () => {
    if (!busy) onAddNew();
  };
  useEffect(() => {
    (hasCandidates ? inputRef : cancelRef).current?.focus();
  }, [hasCandidates]);

  return (
    <div
      role="group"
      aria-label={`${ROLE_TEXT[role].add} for ${personName}`}
      className={styles.picker}
      onKeyDown={(event) => {
        // the search box handles its own Escape (it cancels the default and calls onEscape when
        // there is nothing left to close); from any other control inside the picker, Escape closes it
        if (event.key === 'Escape' && !event.defaultPrevented) cancel();
      }}
    >
      {role === 'partner' ? (
        <div className={styles.kind}>
          <label htmlFor={kindId}>Kind of partnership</label>{' '}
          <select
            id={kindId}
            value={kind}
            onChange={(event) => {
              if (isKind(event.target.value)) onKind(event.target.value);
            }}
          >
            {KINDS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </div>
      ) : null}
      {hasCandidates ? (
        <SearchBox
          persons={candidates}
          onChoose={(id) => {
            if (!busy) onChoose(id);
          }}
          inputRef={inputRef}
          label={ROLE_TEXT[role].find}
          inline
          onEscape={cancel}
        />
      ) : (
        <p className={styles.muted}>No one else in the tree can be added here.</p>
      )}
      <div className={styles.pickerButtons}>
        <button type="button" onClick={addNew} aria-disabled={busy}>
          Add a new person as {role}…
        </button>{' '}
        <button type="button" ref={cancelRef} onClick={cancel} aria-disabled={busy}>
          Cancel
        </button>
      </div>
    </div>
  );
}

/**
 * The person's parents, partners and children, each with the actions that change them. Choosing who
 * to link offers only people the server would accept; the server's answer is still what counts.
 * Nothing changes on screen until a write has succeeded and the graph has been fetched again.
 */
export function Relationships({ family, person, onGoTo, onAddNew, onWritten }: Props) {
  const { parents, partners, children, partnerKinds } = relativesOf(family, person.id);
  const name = displayName(person);
  const [picker, setPicker] = useState<Role | null>(null);
  const [newKind, setNewKind] = useState<PartnerKind>('marriage');
  const [failure, setFailure] = useState<{ error: unknown } | null>(null);

  const addParent = useAddParent();
  const removeParent = useRemoveParent();
  const addPartner = useAddPartner();
  const updatePartner = useUpdatePartner();
  const removePartner = useRemovePartner();
  const busy = [addParent, removeParent, addPartner, updatePartner, removePartner].some(
    (mutation) => mutation.isPending,
  );

  const openers = {
    parent: useRef<HTMLButtonElement>(null),
    partner: useRef<HTMLButtonElement>(null),
    child: useRef<HTMLButtonElement>(null),
  };
  // After cancelling, focus goes back to the button that opened the picker.
  const restoreOpener = useRef<Role | null>(null);
  useEffect(() => {
    const role = restoreOpener.current;
    if (role === null || picker !== null) return;
    restoreOpener.current = null;
    openers[role].current?.focus();
  });

  const candidates = useMemo(() => {
    switch (picker) {
      case 'parent':
        return parentCandidates(family, person.id);
      case 'child':
        return childCandidates(family, person.id);
      case 'partner':
        return partnerCandidates(family, person.id);
      case null:
        return [];
    }
  }, [picker, family, person.id]);

  /** Runs one write: nothing while another is running; shows the failure; moves focus on success. */
  function run(write: () => Promise<unknown>) {
    if (busy) return;
    setFailure(null);
    write().then(
      () => {
        setPicker(null);
        onWritten();
      },
      (error: unknown) => {
        setFailure({ error });
      },
    );
  }

  function openPicker(role: Role) {
    setFailure(null);
    setPicker(role);
  }

  function closePicker(role: Role) {
    restoreOpener.current = role;
    setPicker(null);
  }

  function renderPicker(role: Role, onChoose: (id: number) => void) {
    if (picker !== role) return null;
    return (
      <Picker
        role={role}
        personName={name}
        candidates={candidates}
        kind={newKind}
        onKind={setNewKind}
        busy={busy}
        onChoose={onChoose}
        onAddNew={() => {
          onAddNew(role, newKind);
        }}
        onCancel={() => {
          closePicker(role);
        }}
      />
    );
  }

  const addButton = (role: Role) => (
    <button
      type="button"
      ref={openers[role]}
      aria-disabled={busy || picker === role}
      onClick={() => {
        if (!busy && picker !== role) openPicker(role);
      }}
    >
      {ROLE_TEXT[role].add}
    </button>
  );

  const link = (relative: Person) => (
    <button
      type="button"
      className={styles.link}
      onClick={() => {
        onGoTo(relative.id);
      }}
    >
      {personLabel(relative)}
    </button>
  );

  const removeButton = (label: string, onRemove: () => void) => (
    <button type="button" aria-label={label} aria-disabled={busy} onClick={onRemove}>
      Remove
    </button>
  );

  return (
    <div className={styles.relations}>
      <h3>Parents</h3>
      {parents.length === 0 ? (
        <p className={styles.muted}>None recorded</p>
      ) : (
        <ul className={styles.list}>
          {parents.map((parent) => (
            <li key={parent.id}>
              {link(parent)}{' '}
              {removeButton(`Remove ${displayName(parent)} as a parent of ${name}`, () => {
                run(() => removeParent.mutateAsync({ childId: person.id, parentId: parent.id }));
              })}
            </li>
          ))}
        </ul>
      )}
      {parents.length < MAX_PARENTS ? addButton('parent') : null}
      {renderPicker('parent', (id) => {
        run(() => addParent.mutateAsync({ childId: person.id, parentId: id }));
      })}

      <h3>Partners</h3>
      {partners.length === 0 ? (
        <p className={styles.muted}>None recorded</p>
      ) : (
        <ul className={styles.list}>
          {partners.map((partner) => {
            const kind = partnerKinds.get(partner.id) ?? null;
            const partnerName = displayName(partner);
            return (
              <li key={partner.id}>
                {link(partner)}{' '}
                {kind === null ? (
                  // linked only by a shared child: no partnership is recorded to change or remove
                  <span className={styles.muted}>(shared child)</span>
                ) : (
                  <>
                    <select
                      aria-label={`Kind of partnership with ${partnerName}`}
                      aria-disabled={busy}
                      value={kind}
                      onChange={(event) => {
                        const next = event.target.value;
                        if (isKind(next)) {
                          run(() =>
                            updatePartner.mutateAsync({
                              id: person.id,
                              partnerId: partner.id,
                              kind: next,
                            }),
                          );
                        }
                      }}
                    >
                      {KINDS.map((option) => (
                        <option key={option.value} value={option.value}>
                          {option.label}
                        </option>
                      ))}
                    </select>{' '}
                    {removeButton(`Remove partnership with ${partnerName}`, () => {
                      run(() =>
                        removePartner.mutateAsync({ id: person.id, partnerId: partner.id }),
                      );
                    })}
                  </>
                )}
              </li>
            );
          })}
        </ul>
      )}
      {addButton('partner')}
      {renderPicker('partner', (id) => {
        run(() => addPartner.mutateAsync({ id: person.id, partnerId: id, kind: newKind }));
      })}

      <h3>Children</h3>
      {children.length === 0 ? (
        <p className={styles.muted}>None recorded</p>
      ) : (
        <ul className={styles.list}>
          {children.map((child) => (
            <li key={child.id}>
              {link(child)}{' '}
              {removeButton(`Remove ${displayName(child)} as a child of ${name}`, () => {
                run(() => removeParent.mutateAsync({ childId: child.id, parentId: person.id }));
              })}
            </li>
          ))}
        </ul>
      )}
      {addButton('child')}
      {renderPicker('child', (id) => {
        run(() => addParent.mutateAsync({ childId: id, parentId: person.id }));
      })}

      {failure ? (
        // run() clears the failure before each attempt, so a repeat of the same failure is a new alert
        <div role="alert" className={styles.error}>
          {describeWriteError(failure.error)}
        </div>
      ) : null}
    </div>
  );
}
