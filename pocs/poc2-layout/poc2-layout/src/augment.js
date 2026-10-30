'use strict';

/**
 * Domain -> layout graph augmentation for genealogical layered layout.
 *
 * This is the ONLY place the two graphs meet. Input is pure domain data
 * (persons, parentChild, partnerships); output is an ELK graph whose structure
 * exists solely to drive layout. Isolating this is ADR-002's "decouple layout
 * from domain" made concrete: the renderer never sees a partnership, and the
 * domain never sees a union node.
 */

const PERSON_W_MIN = 84;
const PERSON_H = 40;
const UNION_SIZE = 14;

// A union is identified by its unordered pair of partners, so U(a,b) == U(b,a)
// and we never create the same union twice.
const unionKey = (a, b) => [a, b].sort().join('~');

const personWidth = (name) => Math.max(PERSON_W_MIN, name.length * 9 + 24);

/**
 * Build union nodes and the edges around them. Unions come from two sources,
 * deduped by canonical key:
 *   (1) every registered partnership — even childless ones (your decision);
 *   (2) any co-parent pair with a child that ISN'T a registered partnership
 *       (co-parenting without marriage). None in our fixture, but handled so
 *       the code is correct on real data.
 *
 * Edge DIRECTION encodes the layout intent:
 *   partner -> union   (both partners feed into the union)
 *   union   -> child   (children descend from the union)
 * That direction is what makes elkjs put unions between the two generations.
 */
function buildUnions(data) {
  const persons = new Map(data.persons.map((p) => [p.id, p]));
  const unions = new Map();
  const edges = [];
  const warnings = [];
  let uSeq = 0;
  let eSeq = 0;

  const ensureUnion = (a, b) => {
    const key = unionKey(a, b);
    if (!unions.has(key)) {
      const id = `U${++uSeq}`;
      unions.set(key, { id, a, b, key });
      edges.push({ id: `e${++eSeq}`, sources: [a], targets: [id], kind: 'partner' });
      edges.push({ id: `e${++eSeq}`, sources: [b], targets: [id], kind: 'partner' });
    }
    return unions.get(key);
  };

  // (1) partnerships -> a union each
  for (const [a, b] of data.partnerships) {
    if (!persons.has(a) || !persons.has(b)) {
      warnings.push(`partnership references unknown person: ${a}, ${b}`);
      continue;
    }
    ensureUnion(a, b);
  }

  // group each child's parents together
  const parentsOf = new Map(); // childId -> [parentId, ...]
  for (const [parent, child] of data.parentChild) {
    if (!persons.has(parent) || !persons.has(child)) {
      warnings.push(`parentChild references unknown person: ${parent} -> ${child}`);
      continue;
    }
    if (!parentsOf.has(child)) parentsOf.set(child, []);
    parentsOf.get(child).push(parent);
  }

  // (2) attach children to the union of their parent-set
  for (const [child, parents] of parentsOf) {
    if (parents.length >= 2) {
      if (parents.length > 2) warnings.push(`person ${child} has >2 parents; using first two`);
      const u = ensureUnion(parents[0], parents[1]); // synthesizes a co-parent union if needed
      edges.push({ id: `e${++eSeq}`, sources: [u.id], targets: [child], kind: 'descent' });
    } else {
      // single known parent (incomplete data) -> attach directly, no union
      edges.push({ id: `e${++eSeq}`, sources: [parents[0]], targets: [child], kind: 'descent' });
    }
  }

  return { unions, edges, warnings };
}

// elkjs knobs that matter for genealogy. Each is a deliberate choice, not a
// default — we'll tune these after seeing the first render.
const LAYOUT_OPTIONS = {
  'elk.algorithm': 'layered',
  'elk.direction': 'DOWN',
  // Layer assignment. NETWORK_SIMPLEX minimizes total edge length, which pulls
  // a married-in spouse DOWN onto their partner's generation row instead of
  // stranding them up near the roots.
  'elk.layered.layering.strategy': 'NETWORK_SIMPLEX',
  // Crossing minimization. LAYER_SWEEP is the barycenter/median heuristic
  // (Sugiyama's ordering phase). This is the NP-hard step done approximately.
  'elk.layered.crossingMinimization.strategy': 'LAYER_SWEEP',
  // Coordinate assignment. BRANDES_KOEPF straightens edges and tends to center
  // a union over its children.
  'elk.layered.nodePlacement.strategy': 'BRANDES_KOEPF',
  // Weak tiebreaker: keep the input order (we feed persons birth-year-sorted),
  // so siblings tend to read oldest-to-youngest left-to-right.
  'elk.layered.considerModelOrder.strategy': 'NODES',
  'elk.layered.spacing.nodeNodeBetweenLayers': '45',
  'elk.spacing.nodeNode': '28',
};

/**
 * Build the full ELK graph from domain data.
 * @param {object} overrides - { layoutOptions?: object, partnerPriority?: number }
 *   layoutOptions are merged over the defaults; partnerPriority, if set, tags
 *   every partner edge with an ELK priority (higher = kept shorter/straighter).
 */
function augment(data, overrides = {}) {
  const { unions, edges, warnings } = buildUnions(data);

  if (overrides.partnerPriority != null) {
    for (const e of edges) {
      if (e.kind === 'partner') e.layoutOptions = { 'elk.layered.priority': String(overrides.partnerPriority) };
    }
  }
  const layoutOptions = { ...LAYOUT_OPTIONS, ...(overrides.layoutOptions || {}) };

  const personNodes = [...data.persons]
    .sort((p, q) => (p.birthYear ?? 0) - (q.birthYear ?? 0))
    .map((p) => ({
      id: p.id,
      width: personWidth(p.name),
      height: PERSON_H,
      labels: [{ text: p.name }],
      _person: p,
    }));

  const unionNodes = [...unions.values()].map((u) => ({
    id: u.id,
    width: UNION_SIZE,
    height: UNION_SIZE,
    _union: u,
  }));

  return {
    graph: {
      id: 'root',
      layoutOptions,
      children: [...personNodes, ...unionNodes],
      edges,
    },
    meta: {
      unions,
      warnings,
      personCount: personNodes.length,
      unionCount: unionNodes.length,
      edgeCount: edges.length,
    },
  };
}

module.exports = { augment, buildUnions, unionKey, LAYOUT_OPTIONS };
