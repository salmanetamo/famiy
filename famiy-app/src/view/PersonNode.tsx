import { memo } from 'react';
import type { Person } from '../domain/family';
import { displayName, lifespan, personLabel, truncate } from '../domain/format';
import type { PositionedPerson } from '../layout/types';
import { cx } from './cx';
import styles from './GraphView.module.css';

/** Characters that fit on a 160px card at the card font size; longer names are cut (full name in the tooltip). */
const NAME_CHARS = 19;

interface Props {
  person: Person;
  box: PositionedPerson;
  selected: boolean;
  /** Roving tabindex: exactly one person in the graph is a tab stop. */
  tabbable: boolean;
  /** Zoomed so far out that text cannot be read: draw only the rectangle. */
  compact: boolean;
}

/**
 * One person card. It is memoised, so selecting someone redraws two cards, not the whole graph.
 * Clicks and keys are handled once on the graph (event delegation) via `data-person-id`.
 */
export const PersonNode = memo(function PersonNode({
  person,
  box,
  selected,
  tabbable,
  compact,
}: Props) {
  const name = displayName(person);
  const span = lifespan(person);
  return (
    <g
      className={cx(styles.person, selected && styles.selected)}
      transform={`translate(${String(box.x)} ${String(box.y)})`}
      data-person-id={person.id}
      role="button"
      tabIndex={tabbable ? 0 : -1}
      aria-label={personLabel(person)}
      aria-pressed={selected}
    >
      <rect width={box.width} height={box.height} rx={8} />
      {compact ? null : (
        <>
          <title>{span ? `${name} (${span})` : name}</title>
          <text
            className={styles.name}
            x={box.width / 2}
            y={box.height / 2 - 4}
            textAnchor="middle"
          >
            {truncate(name, NAME_CHARS)}
          </text>
          <text
            className={styles.span}
            x={box.width / 2}
            y={box.height / 2 + 14}
            textAnchor="middle"
          >
            {span}
          </text>
        </>
      )}
    </g>
  );
});
