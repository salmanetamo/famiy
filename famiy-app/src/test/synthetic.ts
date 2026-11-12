import type { ApiGraph } from '../api/types';

/**
 * A deterministic family of `count` people for tests that need a tree too big to see at once:
 * couples with one to three children, most children partnered. The people are called "Person N" and
 * numbered 1..count, so a test can ask for one by name. Its layout is only a few generations deep and
 * very wide, which is what makes most people lie far from any one view.
 */
export function syntheticApiGraph(count: number): ApiGraph {
  let seed = 7;
  const random = () => (seed = (seed * 1664525 + 1013904223) >>> 0) / 4294967296;
  const persons: ApiGraph['persons'] = [];
  const parent_links: ApiGraph['parent_links'] = [];
  const partnerships: ApiGraph['partnerships'] = [];
  const make = (year: number) => {
    const id = persons.length + 1;
    persons.push({
      id,
      given_name: `Person ${String(id)}`,
      family_name: null,
      birth_date: `${String(year)}-01-01`,
      birth_approx: true,
      death_date: null,
    });
    return { id, year };
  };
  const queue: [{ id: number; year: number }, { id: number; year: number }][] = [];
  while (persons.length < Math.max(2, count * 0.1)) {
    const a = make(1850 + Math.floor(random() * 10));
    const b = make(a.year + 2);
    partnerships.push({ partners: [a.id, b.id], kind: 'marriage' });
    queue.push([a, b]);
  }
  for (
    let next = queue.shift();
    next !== undefined && persons.length < count;
    next = queue.shift()
  ) {
    const [a, b] = next;
    const kids = 1 + Math.floor(random() * 3);
    for (let i = 0; i < kids && persons.length < count; i++) {
      const child = make(Math.max(a.year, b.year) + 25 + i * 2);
      parent_links.push(
        { parent_id: a.id, child_id: child.id },
        { parent_id: b.id, child_id: child.id },
      );
      if (persons.length < count && random() < 0.8) {
        const spouse = make(child.year + Math.floor(random() * 6) - 3);
        partnerships.push({ partners: [child.id, spouse.id], kind: 'marriage' });
        queue.push([child, spouse]);
      }
    }
  }
  return { persons, parent_links, partnerships };
}
