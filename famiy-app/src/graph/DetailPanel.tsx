import { useId } from 'react';
import { describeApiError } from '../api/describeError';
import { relativesOf, type FamilyGraph, type Person } from '../domain/family';
import { displayName, formatDate, lifespan, personLabel } from '../domain/format';
import styles from './DetailPanel.module.css';
import { usePersonDetail } from './usePersonDetail';

interface Props {
  family: FamilyGraph;
  person: Person;
  onCenter: () => void;
  onClear: () => void;
  /** Select another person (a relative) and bring them into view. */
  onGoTo: (id: number) => void;
}

function RelativeList({
  title,
  people,
  onGoTo,
}: {
  title: string;
  people: readonly Person[];
  onGoTo: (id: number) => void;
}) {
  if (people.length === 0) return null;
  return (
    <>
      <dt>{title}</dt>
      <dd>
        <ul className={styles.relatives}>
          {people.map((relative) => (
            <li key={relative.id}>
              <button
                type="button"
                className={styles.link}
                onClick={() => {
                  onGoTo(relative.id);
                }}
              >
                {personLabel(relative)}
              </button>
            </li>
          ))}
        </ul>
      </dd>
    </>
  );
}

/**
 * Everything known about the selected person. The summary and relatives come from the graph that is
 * already loaded, so the panel is useful immediately; only the bio and residence are fetched.
 * Bio is free text: it is shown as plain text, never interpreted as markup.
 */
export function DetailPanel({ family, person, onCenter, onClear, onGoTo }: Props) {
  const titleId = useId();
  const detail = usePersonDetail(person.id);
  const { parents, partners, children } = relativesOf(family, person.id);
  const span = lifespan(person);
  const hasRelatives = parents.length + partners.length + children.length > 0;

  return (
    <section className={styles.panel} aria-labelledby={titleId}>
      <h2 id={titleId} className={styles.title}>
        {displayName(person)}
      </h2>
      {span ? <p className={styles.span}>{span}</p> : null}
      <div className={styles.actions}>
        <button type="button" onClick={onCenter}>
          Center
        </button>{' '}
        <button type="button" onClick={onClear}>
          Clear
        </button>
      </div>

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
        <RelativeList title="Parents" people={parents} onGoTo={onGoTo} />
        <RelativeList title="Partners" people={partners} onGoTo={onGoTo} />
        <RelativeList title="Children" people={children} onGoTo={onGoTo} />
      </dl>

      {!hasRelatives ? <p className={styles.muted}>No relatives are recorded yet.</p> : null}
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
    </section>
  );
}
