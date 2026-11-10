import { useEffect, useRef, useState } from 'react';
import type { PartnerKind } from '../api/types';
import { emptyPersonForm, toCreateBody } from '../domain/personForm';
import styles from './DetailPanel.module.css';
import { PersonForm } from './PersonForm';
import type { Role } from './Relationships';
import { useCreatePerson } from './usePersonMutations';
import { useAddParent, useAddPartner } from './useRelationshipMutations';

/** The person this new one is being added for, and how they will be linked. */
export interface Relation {
  role: Role;
  /** The kind of partnership to record when `role` is partner. */
  kind: PartnerKind;
  ofId: number;
  ofName: string;
}

export interface CreateOutcome {
  /** This form was still open, so the screen should close it and show the new person. */
  open: boolean;
  /** Set when the person was saved but linking them failed (they exist; they are just not linked). */
  linkError?: unknown;
  relation?: Relation | undefined;
}

interface Props {
  /** Set when the new person is to be linked to someone as soon as they are saved. */
  relation?: Relation | undefined;
  /**
   * Called with the new person's id once they are saved, linked and the graph is refreshed, even if the
   * user has moved on meanwhile (`open` says whether this form was still showing).
   */
  onCreated: (id: number, outcome: CreateOutcome) => void;
  onCancel: () => void;
}

const TITLES: Record<Role, string> = {
  parent: 'Add a parent of',
  partner: 'Add a partner of',
  child: 'Add a child of',
};

/** Add a person, optionally linking them straight away as a parent, partner or child of someone. */
export function AddPersonPanel({ relation, onCreated, onCancel }: Props) {
  const create = useCreatePerson();
  const addParent = useAddParent();
  const addPartner = useAddPartner();
  const [linking, setLinking] = useState(false);
  // The save keeps running if the user moves on (the graph is refreshed regardless); only the
  // follow-up that selects the new person must not run once this form is gone.
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  /** Links the new person. */
  function link(newId: number, to: Relation): Promise<unknown> {
    switch (to.role) {
      case 'parent':
        return addParent.mutateAsync({ childId: to.ofId, parentId: newId });
      case 'child':
        return addParent.mutateAsync({ childId: newId, parentId: to.ofId });
      case 'partner':
        return addPartner.mutateAsync({ id: to.ofId, partnerId: newId, kind: to.kind });
    }
  }

  const title = relation ? `${TITLES[relation.role]} ${relation.ofName}` : 'Add a person';

  return (
    <section className={styles.panel} aria-labelledby="add-person-title">
      <h2 id="add-person-title" className={styles.title}>
        {title}
      </h2>
      <PersonForm
        initial={emptyPersonForm}
        submitLabel="Add person"
        pending={create.isPending || linking}
        error={create.error}
        hasError={create.isError}
        onSubmit={(values) => {
          create.mutateAsync(toCreateBody(values)).then(
            async (person) => {
              let linkError: unknown;
              if (relation) {
                setLinking(true);
                try {
                  await link(person.id, relation);
                } catch (error) {
                  linkError = error;
                }
              }
              const open = mounted.current;
              if (open) setLinking(false);
              onCreated(person.id, { open, linkError, relation });
            },
            () => undefined, // shown by the form through `create.error`
          );
        }}
        onCancel={onCancel}
      />
    </section>
  );
}
