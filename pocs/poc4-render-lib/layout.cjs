'use strict';
// Produces public/layout-small.json (PoC-2 fixture) and public/layout-1k.json (synthetic 1,000 persons)
// using PoC-2's augmentWithContainers + elkjs. Both renderer candidates draw from the SAME output.
const fs = require('fs');
const path = require('path');
const ELK = require('elkjs');
const P2 = path.join(__dirname, '..', 'poc2-layout', 'poc2-layout');
const { augmentWithContainers } = require(path.join(P2, 'src', 'augment-containers.js'));
const { flattenNodes } = require(path.join(P2, 'src', 'flatten.js'));

function rng(seed) { let s = seed >>> 0; return () => ((s = (s * 1664525 + 1013904223) >>> 0) / 4294967296); }

// ~1,000 persons: founder couples, each child of a couple has a spouse married in, kids until budget used.
function synth(n, seed = 7) {
  const r = rng(seed); const persons = [], pc = [], parts = []; let id = 0;
  const mk = (year) => { const p = { id: 'x' + ++id, name: 'P' + id, birthYear: year }; persons.push(p); return p; };
  let queue = [];
  while (persons.length < n * 0.1) { const a = mk(1850 + Math.floor(r() * 10)), b = mk(a.birthYear + 2); parts.push([a.id, b.id]); queue.push([a, b]); }
  while (persons.length < n && queue.length) {
    const [a, b] = queue.shift(); const k = 1 + Math.floor(r() * 3);
    for (let i = 0; i < k && persons.length < n; i++) {
      const c = mk(Math.max(a.birthYear, b.birthYear) + 25 + i * 2); pc.push([a.id, c.id], [b.id, c.id]);
      if (persons.length < n && r() < 0.8) { const s = mk(c.birthYear + Math.floor(r() * 6) - 3); parts.push([c.id, s.id]); queue.push([c, s]); }
    }
  }
  return { persons, parentChild: pc, partnerships: parts };
}

async function run(name, data) {
  const { graph, meta } = augmentWithContainers(data);
  const elk = new ELK(); const t0 = performance.now();
  const laid = await elk.layout(graph); const ms = performance.now() - t0;
  const nodes = flattenNodes(laid).map((n) => {
    const raw = n.raw; const kind = n.isContainer ? 'container' : raw._union ? 'union' : 'person';
    return { id: n.id, x: n.x, y: n.y, w: n.width, h: n.height, kind, label: raw.labels?.[0]?.text };
  });
  // edges are listed at the root, so section points are in root coordinates
  const edges = (laid.edges || []).map((e) => {
    const s = e.sections?.[0]; if (!s) return null;
    return { id: e.id, source: e.sources[0], target: e.targets[0], kind: e.kind, pts: [s.startPoint, ...(s.bendPoints || []), s.endPoint].map((p) => [p.x, p.y]) };
  }).filter(Boolean);
  fs.mkdirSync(path.join(__dirname, 'public'), { recursive: true });
  fs.writeFileSync(path.join(__dirname, 'public', `layout-${name}.json`), JSON.stringify({ nodes, edges }));
  console.log(name, 'persons', data.persons.length, 'nodes', nodes.length, 'edges', edges.length, 'containers', meta.containerCount, 'elk layout ms', ms.toFixed(0));
}
(async () => {
  const small = JSON.parse(fs.readFileSync(path.join(P2, 'data', 'messy-family.json'), 'utf8'));
  await run('small', small); await run('1k', synth(1000));
})();
