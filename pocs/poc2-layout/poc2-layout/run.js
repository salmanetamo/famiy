'use strict';

/**
 * PoC-2 entry point: domain data -> augment -> elkjs layout -> SVG + report.
 *   node run.js [path/to/family.json]
 */
const fs = require('fs');
const path = require('path');
const ELK = require('elkjs');
const { augment } = require('./src/augment');
const { toSvg } = require('./src/render-svg');

// Approximate crossing count: treat each edge as the straight segment between
// the centers of its endpoint nodes, and count intersecting pairs that don't
// share an endpoint. A proxy (ignores orthogonal routing), but a stable number
// to watch as we tune layout options.
function orient(ax, ay, bx, by, cx, cy) {
  const v = (by - ay) * (cx - bx) - (bx - ax) * (cy - by);
  return v > 1e-9 ? 1 : v < -1e-9 ? -1 : 0;
}
function segmentsCross(p1, p2, p3, p4) {
  const o1 = orient(p1.x, p1.y, p2.x, p2.y, p3.x, p3.y);
  const o2 = orient(p1.x, p1.y, p2.x, p2.y, p4.x, p4.y);
  const o3 = orient(p3.x, p3.y, p4.x, p4.y, p1.x, p1.y);
  const o4 = orient(p3.x, p3.y, p4.x, p4.y, p2.x, p2.y);
  return o1 !== o2 && o3 !== o4;
}
function countCrossings(laid) {
  const center = new Map(laid.children.map((n) => [n.id, { x: n.x + n.width / 2, y: n.y + n.height / 2 }]));
  const segs = (laid.edges || []).map((e) => ({
    a: e.sources[0], b: e.targets[0],
    p: center.get(e.sources[0]), q: center.get(e.targets[0]),
  }));
  let n = 0;
  for (let i = 0; i < segs.length; i++) {
    for (let j = i + 1; j < segs.length; j++) {
      const s = segs[i], t = segs[j];
      if (s.a === t.a || s.a === t.b || s.b === t.a || s.b === t.b) continue; // share a node
      if (segmentsCross(s.p, s.q, t.p, t.q)) n++;
    }
  }
  return n;
}

(async () => {
  const dataPath = process.argv[2] || path.join(__dirname, 'data', 'messy-family.json');
  const data = JSON.parse(fs.readFileSync(dataPath, 'utf8'));
  const { graph, meta } = augment(data);

  const elk = new ELK();
  const laid = await elk.layout(graph);

  const svg = toSvg(laid, data, meta);
  const outPath = path.join(__dirname, 'out', 'messy-family.svg');
  fs.writeFileSync(outPath, svg);

  // report: generation rows + which persons landed on each
  const persons = new Map(data.persons.map((p) => [p.id, p]));
  const personNodes = laid.children.filter((n) => persons.has(n.id));
  const rowYs = [...new Set(personNodes.map((n) => Math.round(n.y)))].sort((a, b) => a - b);
  const rowIndex = new Map(rowYs.map((y, i) => [y, i]));

  console.log(`nodes=${laid.children.length}  edges=${laid.edges.length}  size=${Math.ceil(laid.width)}x${Math.ceil(laid.height)}`);
  console.log(`approx crossings (straight-segment proxy): ${countCrossings(laid)}`);
  console.log('\ngeneration rows (by layout y):');
  for (const y of rowYs) {
    const names = personNodes
      .filter((n) => Math.round(n.y) === y)
      .sort((a, b) => a.x - b.x)
      .map((n) => persons.get(n.id).name);
    console.log(`  Gen ${rowIndex.get(y)} (y=${y}): ${names.join(', ')}`);
  }
  console.log(`\nwrote ${outPath}`);
})().catch((err) => { console.error(err); process.exit(1); });
