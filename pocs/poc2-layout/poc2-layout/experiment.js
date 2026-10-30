'use strict';

/**
 * PoC-2 tuning sweep: try several automatic layout configs on the same fixture
 * and measure which best keeps couples adjacent without adding crossings.
 *
 * Metrics (lower is better):
 *   maxCoupleSpan  - the widest horizontal gap between any two partners (px).
 *                    ~node width means "adjacent"; hundreds means "split apart".
 *   crossings      - straight-segment proxy, excluding edges sharing a node.
 *   totalEdgeLen   - sum of straight edge lengths (secondary tiebreaker).
 */
const fs = require('fs');
const path = require('path');
const ELK = require('elkjs');
const { augment } = require('./src/augment');
const { augmentWithContainers } = require('./src/augment-containers');
const { toSvg } = require('./src/render-svg');
const { flattenNodes } = require('./src/flatten');

const data = JSON.parse(fs.readFileSync(path.join(__dirname, 'data', 'messy-family.json'), 'utf8'));

function orient(ax, ay, bx, by, cx, cy) {
  const v = (by - ay) * (cx - bx) - (bx - ax) * (cy - by);
  return v > 1e-9 ? 1 : v < -1e-9 ? -1 : 0;
}
const cross = (p1, p2, p3, p4) =>
  orient(p1.x, p1.y, p2.x, p2.y, p3.x, p3.y) !== orient(p1.x, p1.y, p2.x, p2.y, p4.x, p4.y) &&
  orient(p3.x, p3.y, p4.x, p4.y, p1.x, p1.y) !== orient(p3.x, p3.y, p4.x, p4.y, p2.x, p2.y);

function metrics(laid, meta) {
  // hierarchy-aware: use absolute leaf positions so container variants compare fairly
  const leaves = flattenNodes(laid).filter((n) => !n.isContainer);
  const c = new Map(leaves.map((n) => [n.id, { x: n.x + n.width / 2, y: n.y + n.height / 2 }]));
  // max couple span
  let maxCoupleSpan = 0;
  for (const u of meta.unions.values()) {
    maxCoupleSpan = Math.max(maxCoupleSpan, Math.abs(c.get(u.a).x - c.get(u.b).x));
  }
  // crossings + total length
  const segs = laid.edges.map((e) => ({ a: e.sources[0], b: e.targets[0], p: c.get(e.sources[0]), q: c.get(e.targets[0]) }));
  let crossings = 0, totalEdgeLen = 0;
  for (const s of segs) totalEdgeLen += Math.hypot(s.p.x - s.q.x, s.p.y - s.q.y);
  for (let i = 0; i < segs.length; i++)
    for (let j = i + 1; j < segs.length; j++) {
      const s = segs[i], t = segs[j];
      if (s.a === t.a || s.a === t.b || s.b === t.a || s.b === t.b) continue;
      if (cross(s.p, s.q, t.p, t.q)) crossings++;
    }
  return { maxCoupleSpan: Math.round(maxCoupleSpan), crossings, totalEdgeLen: Math.round(totalEdgeLen) };
}

// Each config is either an { overrides } fed to augment(), or a custom builder.
const CONFIGS = {
  baseline: { build: () => augment(data) },
  modelOrder_nodesEdges: { build: () => augment(data, { layoutOptions: { 'elk.layered.considerModelOrder.strategy': 'NODES_AND_EDGES' } }) },
  partnerPriority_50: { build: () => augment(data, { partnerPriority: 50 }) },
  priority_plus_order: { build: () => augment(data, { partnerPriority: 50, layoutOptions: { 'elk.layered.considerModelOrder.strategy': 'NODES_AND_EDGES' } }) },
  coupleContainers: { build: () => augmentWithContainers(data) },
};

(async () => {
  const elk = new ELK();
  const rows = [];
  const laidByName = {};
  for (const [name, cfg] of Object.entries(CONFIGS)) {
    const { graph, meta } = cfg.build();
    const laid = await elk.layout(graph);
    laidByName[name] = { laid, meta };
    rows.push({ name, ...metrics(laid, meta) });
  }

  rows.sort((a, b) => a.maxCoupleSpan - b.maxCoupleSpan || a.crossings - b.crossings || a.totalEdgeLen - b.totalEdgeLen);

  const h = `${'config'.padEnd(24)}${'maxCoupleSpan'.padStart(15)}${'crossings'.padStart(11)}${'totalEdgeLen'.padStart(14)}`;
  console.log(h);
  console.log('-'.repeat(h.length));
  for (const r of rows) {
    console.log(`${r.name.padEnd(24)}${String(r.maxCoupleSpan).padStart(15)}${String(r.crossings).padStart(11)}${String(r.totalEdgeLen).padStart(14)}`);
  }

  // render the winner
  const best = rows[0];
  const { laid, meta } = laidByName[best.name];
  const svg = toSvg(laid, data, meta);
  fs.writeFileSync(path.join(__dirname, 'out', 'messy-family-tuned.svg'), svg);
  console.log(`\nwinner: ${best.name}  -> wrote out/messy-family-tuned.svg`);
})().catch((e) => { console.error(e); process.exit(1); });
