import { useId, useMemo, useState } from 'react';
import type { Person } from '../domain/family';
import { personLabel } from '../domain/format';
import { buildSearchIndex, searchPeople } from '../domain/search';
import styles from './SearchBox.module.css';

/** How many matches are listed; the rest are summarised, so the list never outgrows the screen. */
export const MAX_SHOWN = 8;

interface Props {
  persons: readonly Person[];
  onChoose: (id: number) => void;
  /** The search input, so the screen can move focus to it (for example after a delete). */
  inputRef?: React.Ref<HTMLInputElement>;
  /** What the box is for, as its accessible name and placeholder. */
  label?: string;
  /** Show results in the page flow instead of floating over what is below (inside a scrolling panel). */
  inline?: boolean;
  /** Escape was pressed with nothing left to close or clear. */
  onEscape?: () => void;
}

/**
 * Find a person by name (ARIA combobox with a listbox). It searches the people already loaded with
 * the graph, so there is no request per keystroke. Typing opens the list, Up/Down move through it,
 * Enter picks, Escape closes it (and, when it is already closed, clears the text, and when there is
 * no text either, tells the owner through `onEscape`).
 */
export function SearchBox({
  persons,
  onChoose,
  inputRef,
  label = 'Find a person',
  inline = false,
  onEscape,
}: Props) {
  const index = useMemo(() => buildSearchIndex(persons), [persons]);
  const [query, setQuery] = useState('');
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const listId = useId();

  const searched = query.trim() !== '';
  const { people, total } = useMemo(() => searchPeople(index, query, MAX_SHOWN), [index, query]);
  const showList = open && searched;
  const active = Math.min(activeIndex, people.length - 1);
  const optionId = (position: number) => `${listId}-${String(position)}`;

  function choose(person: Person) {
    setQuery('');
    setOpen(false);
    setActiveIndex(0);
    onChoose(person.id);
  }

  function onKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      if (!searched || people.length === 0) return;
      setOpen(true);
      const step = event.key === 'ArrowDown' ? 1 : -1;
      setActiveIndex((active + step + people.length) % people.length);
    } else if (event.key === 'Enter') {
      const person = people[active];
      if (showList && person) {
        event.preventDefault();
        choose(person);
      }
    } else if (event.key === 'Escape') {
      // a search input clears itself on Escape by default; the first press must only close the list
      event.preventDefault();
      if (showList) setOpen(false);
      else if (query !== '') setQuery('');
      else onEscape?.();
    }
  }

  const announcement = !searched
    ? ''
    : total === 0
      ? 'No one found'
      : `${String(total)} ${total === 1 ? 'person' : 'people'} found`;

  return (
    <div className={styles.search}>
      <label className="visually-hidden" htmlFor={`${listId}-input`}>
        {label}
      </label>
      <input
        id={`${listId}-input`}
        ref={inputRef}
        type="search"
        role="combobox"
        className={styles.input}
        placeholder={label}
        autoComplete="off"
        spellCheck={false}
        value={query}
        aria-expanded={showList}
        aria-controls={listId}
        aria-autocomplete="list"
        aria-activedescendant={showList && people.length > 0 ? optionId(active) : undefined}
        onChange={(event) => {
          setQuery(event.target.value);
          setActiveIndex(0);
          setOpen(true);
        }}
        onKeyDown={onKeyDown}
        onFocus={() => {
          setOpen(true);
        }}
        onBlur={() => {
          setOpen(false);
        }}
      />
      <div role="status" className="visually-hidden">
        {announcement}
      </div>
      <div className={inline ? styles.popupInline : styles.popup} hidden={!showList}>
        <ul id={listId} role="listbox" aria-label="People found" className={styles.list}>
          {showList
            ? people.map((person, position) => (
                <li
                  key={person.id}
                  id={optionId(position)}
                  role="option"
                  aria-selected={position === active}
                  className={position === active ? styles.activeOption : styles.option}
                  // keep focus in the input, so the list does not close before the click lands
                  onMouseDown={(event) => {
                    event.preventDefault();
                  }}
                  onClick={() => {
                    choose(person);
                  }}
                >
                  {personLabel(person)}
                </li>
              ))
            : null}
        </ul>
        {showList && total === 0 ? (
          <p className={styles.note}>No one matches “{query.trim()}”.</p>
        ) : null}
        {showList && total > people.length ? (
          <p className={styles.note}>
            {String(total - people.length)} more. Keep typing to narrow it down.
          </p>
        ) : null}
      </div>
    </div>
  );
}
