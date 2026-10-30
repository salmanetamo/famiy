'use strict';

/**
 * Variant augmentation: wrap monogamous couples in a COUPLE CONTAINER so elkjs
 * cannot separate the two partners.
 *
 * Why only monogamous couples? A node can belong to exactly one container, but a
 * remarried person belongs to two couples (our Idi is in Idi+Rama and Idi+Zara).
 * Containerizing them is impossible, so we containerize only couples where BOTH
 * partners appear in exactly one union, and leave remarried people at the root
 * (their unions were already adjacent in the baseline anyway).
 *
 * Mechanics: the container is a compound ELK node running its own tiny DOWN
 * layout over two unconnected person nodes, which puts them side by side. The
 * root turns on hierarchyHandling=INCLUDE_CHILDREN so descent/partner edges can
 * cross the container boundary.
 */
const { buildUnions, LAYOUT_OPTIONS } = require('./augment');

const PERSON_W_MIN = 84;
const PERSON_H = 40;
const UNION_SIZE = 14;
const personWidth = (name) => Math.max(PERSON_W_MIN, name.length * 9 + 24);

function augmentWithContainers(data) {
  const { unions, edges, warnings } = buildUnions(data);
  const persons = new Map(data.persons.map((p) => [p.id, p]));
  const byBirth = (a, b) => (persons.get(a).birthYear ?? 0) - (persons.get(b).birthYear ?? 0);

  // how many unions is each person part of?
  const unionCount = new Map();
  for (const u of unions.values()) {
    unionCount.set(u.a, (unionCount.get(u.a) || 0) + 1);
    unionCount.set(u.b, (unionCount.get(u.b) || 0) + 1);
  }

  // pick containerizable couples and record which person goes in which container
  const containerOf = new Map();
  const containers = [];
  let cSeq = 0;
  for (const u of unions.values()) {
    if (unionCount.get(u.a) === 1 && unionCount.get(u.b) === 1) {
      const id = `C${++cSeq}`;
      containers.push({ id, members: [u.a, u.b], union: u });
      containerOf.set(u.a, id);
      containerOf.set(u.b, id);
    }
  }

  const personNode = (p) => ({ id: p.id, width: personWidth(p.name), height: PERSON_H, labels: [{ text: p.name }], _person: p });

  const containerNodes = containers.map((c) => ({
    id: c.id,
    layoutOptions: {
      'elk.algorithm': 'layered',
      'elk.direction': 'DOWN',
      'elk.spacing.nodeNode': '16',
      'elk.padding': '[top=6,left=6,bottom=6,right=6]',
    },
    children: [...c.members].sort(byBirth).map((id) => personNode(persons.get(id))),
  }));

  const rootPersons = [...persons.values()]
    .filter((p) => !containerOf.has(p.id))
    .sort((a, b) => (a.birthYear ?? 0) - (b.birthYear ?? 0))
    .map(personNode);

  const unionNodes = [...unions.values()].map((u) => ({ id: u.id, width: UNION_SIZE, height: UNION_SIZE, _union: u }));

  const layoutOptions = { ...LAYOUT_OPTIONS, 'elk.hierarchyHandling': 'INCLUDE_CHILDREN' };

  return {
    graph: { id: 'root', layoutOptions, children: [...containerNodes, ...rootPersons, ...unionNodes], edges },
    meta: { unions, warnings, containers, containerCount: containers.length },
  };
}

module.exports = { augmentWithContainers };
