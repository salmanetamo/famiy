import { useEffect, useId, useRef, useState, type SyntheticEvent } from 'react';
import { describeWriteError } from '../api/describeError';
import {
  validatePersonForm,
  type PersonFormErrors,
  type PersonFormValues,
} from '../domain/personForm';
import styles from './PersonForm.module.css';

interface Props {
  initial: PersonFormValues;
  submitLabel: string;
  /** True while the save (and the graph refresh after it) is running. */
  pending: boolean;
  /** The last failed save, shown above the buttons. */
  error: unknown;
  hasError: boolean;
  onSubmit: (values: PersonFormValues) => void;
  onCancel: () => void;
}

const FIELD_ERRORS = ['givenName', 'birthDate', 'deathDate'] as const;

/** Which field errors stop being true when a field changes. */
const ERRORS_FIXED_BY: Partial<
  Record<keyof PersonFormValues, readonly (keyof PersonFormErrors)[]>
> = {
  givenName: ['givenName'],
  birthDate: ['birthDate', 'deathDate'],
  birthApprox: ['birthDate'],
  deathDate: ['deathDate'],
};

/** The fields of one person. Controlled; checks only what saves a round trip, the server does the rest. */
export function PersonForm({
  initial,
  submitLabel,
  pending,
  error,
  hasError,
  onSubmit,
  onCancel,
}: Props) {
  const [values, setValues] = useState(initial);
  const [errors, setErrors] = useState<PersonFormErrors>({});
  const formId = useId();
  const id = (field: string) => `${formId}-${field}`;
  const givenRef = useRef<HTMLInputElement>(null);

  // the form opens ready to type in
  useEffect(() => {
    givenRef.current?.focus();
  }, []);

  const set = <K extends keyof PersonFormValues>(field: K, value: PersonFormValues[K]) => {
    setValues((previous) => ({ ...previous, [field]: value }));
    // an error describes what was typed before, so it goes as soon as that field changes
    const stale = ERRORS_FIXED_BY[field];
    if (stale) {
      setErrors((previous) => {
        const next: PersonFormErrors = {};
        for (const key of FIELD_ERRORS) {
          const message = previous[key];
          if (message !== undefined && !stale.includes(key)) next[key] = message;
        }
        return next;
      });
    }
  };

  function submit(event: SyntheticEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) return;
    const found = validatePersonForm(values);
    setErrors(found);
    const first = (['givenName', 'birthDate', 'deathDate'] as const).find((key) => found[key]);
    if (first) {
      document.getElementById(id(first))?.focus();
      return;
    }
    onSubmit(values);
  }

  const field = (key: keyof PersonFormErrors) => ({
    'aria-invalid': errors[key] ? true : undefined,
    'aria-describedby': errors[key] ? id(`${key}-error`) : undefined,
  });
  const fieldError = (key: keyof PersonFormErrors) =>
    errors[key] ? (
      <p id={id(`${key}-error`)} className={styles.fieldError}>
        {errors[key]}
      </p>
    ) : null;

  return (
    <form
      className={styles.form}
      onSubmit={submit}
      noValidate
      onKeyDown={(event) => {
        if (event.key === 'Escape' && !pending) onCancel();
      }}
    >
      <label htmlFor={id('givenName')}>Given name (required)</label>
      <input
        id={id('givenName')}
        ref={givenRef}
        autoComplete="off"
        value={values.givenName}
        onChange={(event) => {
          set('givenName', event.target.value);
        }}
        {...field('givenName')}
      />
      {fieldError('givenName')}

      <label htmlFor={id('familyName')}>Family name</label>
      <input
        id={id('familyName')}
        autoComplete="off"
        value={values.familyName}
        onChange={(event) => {
          set('familyName', event.target.value);
        }}
      />

      <label htmlFor={id('birthDate')}>Birth date</label>
      <input
        id={id('birthDate')}
        type="date"
        value={values.birthDate}
        onChange={(event) => {
          set('birthDate', event.target.value);
        }}
        {...field('birthDate')}
      />
      <label className={styles.check}>
        <input
          type="checkbox"
          checked={values.birthApprox}
          onChange={(event) => {
            set('birthApprox', event.target.checked);
          }}
        />{' '}
        Birth date is approximate
      </label>
      {fieldError('birthDate')}

      <label htmlFor={id('deathDate')}>Death date</label>
      <input
        id={id('deathDate')}
        type="date"
        value={values.deathDate}
        onChange={(event) => {
          set('deathDate', event.target.value);
        }}
        {...field('deathDate')}
      />
      {fieldError('deathDate')}

      <label htmlFor={id('residence')}>Residence</label>
      <input
        id={id('residence')}
        autoComplete="off"
        value={values.residence}
        onChange={(event) => {
          set('residence', event.target.value);
        }}
      />

      <label htmlFor={id('bio')}>Notes</label>
      <textarea
        id={id('bio')}
        rows={4}
        maxLength={2000}
        value={values.bio}
        onChange={(event) => {
          set('bio', event.target.value);
        }}
      />

      {hasError ? (
        <div role="alert" className={styles.serverError}>
          {describeWriteError(error)}
        </div>
      ) : null}

      <div className={styles.buttons}>
        <button type="submit" aria-disabled={pending}>
          {pending ? 'Saving…' : submitLabel}
        </button>{' '}
        <button type="button" onClick={onCancel} disabled={pending}>
          Cancel
        </button>
      </div>
    </form>
  );
}
