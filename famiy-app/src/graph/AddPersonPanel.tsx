import { useEffect, useRef } from 'react';
import { emptyPersonForm, toCreateBody } from '../domain/personForm';
import styles from './DetailPanel.module.css';
import { PersonForm } from './PersonForm';
import { useCreatePerson } from './usePersonMutations';

interface Props {
  /** Called with the new person's id once they are saved and the graph is refreshed. */
  onCreated: (id: number) => void;
  onCancel: () => void;
}

/** Add a person with no relatives; linking them to others is done from their panel. */
export function AddPersonPanel({ onCreated, onCancel }: Props) {
  const create = useCreatePerson();
  // The save keeps running if the user moves on (the graph is refreshed regardless); only the
  // follow-up that selects the new person must not run once this form is gone.
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  return (
    <section className={styles.panel} aria-labelledby="add-person-title">
      <h2 id="add-person-title" className={styles.title}>
        Add a person
      </h2>
      <PersonForm
        initial={emptyPersonForm}
        submitLabel="Add person"
        pending={create.isPending}
        error={create.error}
        hasError={create.isError}
        onSubmit={(values) => {
          create.mutateAsync(toCreateBody(values)).then(
            (person) => {
              if (mounted.current) onCreated(person.id);
            },
            () => undefined, // shown by the form through `create.error`
          );
        }}
        onCancel={onCancel}
      />
    </section>
  );
}
