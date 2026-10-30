'use strict';

/**
 * Minimal SVG renderer for a laid-out ELK graph. Deliberately diagnostic:
 *  - faint horizontal bands + "Gen N" labels, so "do generations align?" is
 *    answerable by eye (criterion #1);
 *  - persons as rounded rects tinted by sex, unions as small dark dots;
 *  - edges drawn along ELK's routed sections (orthogonal).
 * This renderer is throwaway PoC scaffolding, not the eventual React/SVG view.
 */

const { flattenNodes } = require('./flatten');

function bbox(nodes) {
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  for (const n of nodes) {
    minX = Math.min(minX, n.x);
    minY = Math.min(minY, n.y);
    maxX = Math.max(maxX, n.x + n.width);
    maxY = Math.max(maxY, n.y + n.height);
  }
  return { minX, minY, maxX, maxY };
}

function edgePoints(edge) {
  const pts = [];
  for (const s of edge.sections || []) {
    pts.push([s.startPoint.x, s.startPoint.y]);
    for (const b of s.bendPoints || []) pts.push([b.x, b.y]);
    pts.push([s.endPoint.x, s.endPoint.y]);
  }
  return pts;
}

const esc = (s) => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

function toSvg(laid, data, meta) {
  const persons = new Map(data.persons.map((p) => [p.id, p]));
  const flat = flattenNodes(laid);                 // absolute positions, hierarchy-aware
  const nodes = flat.filter((n) => !n.isContainer); // persons + unions
  const containers = flat.filter((n) => n.isContainer);
  const personNodes = nodes.filter((n) => persons.has(n.id));

  const pad = 44;
  const B = bbox(nodes);
  const ox = pad - B.minX;
  const oy = pad - B.minY;
  const W = (B.maxX - B.minX) + pad * 2;
  const H = (B.maxY - B.minY) + pad * 2;

  // generation rows: cluster person nodes by y with a tolerance, so that a few
  // px of container-padding offset doesn't split one visual row into several.
  const ROW_TOL = 30;
  const sortedY = [...personNodes.map((n) => n.y)].sort((a, b) => a - b);
  const rowYs = [];
  for (const y of sortedY) {
    if (!rowYs.length || y - rowYs[rowYs.length - 1] > ROW_TOL) rowYs.push(y);
  }

  const out = [];
  out.push(`<svg xmlns="http://www.w3.org/2000/svg" width="${Math.ceil(W)}" height="${Math.ceil(H)}" font-family="system-ui,sans-serif">`);
  out.push(`<rect width="100%" height="100%" fill="#ffffff"/>`);

  // generation bands + labels
  rowYs.forEach((y, i) => {
    const yc = y + oy;
    const bandH = 40 + 24;
    if (i % 2 === 1) out.push(`<rect x="0" y="${yc - 12}" width="${Math.ceil(W)}" height="${bandH}" fill="#f1f5f9"/>`);
    out.push(`<text x="10" y="${yc + 24}" font-size="11" fill="#94a3b8">Gen ${i}</text>`);
  });

  // couple containers (faint outline, drawn under everything)
  for (const c of containers) {
    out.push(`<rect x="${(c.x + ox).toFixed(1)}" y="${(c.y + oy).toFixed(1)}" width="${c.width}" height="${c.height}" rx="10" fill="#fafafa" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="4 3"/>`);
  }

  // edges (under nodes)
  for (const e of laid.edges || []) {
    const pts = edgePoints(e).map(([x, y]) => `${(x + ox).toFixed(1)},${(y + oy).toFixed(1)}`).join(' ');
    const partner = (e.kind === 'partner');
    out.push(`<polyline points="${pts}" fill="none" stroke="${partner ? '#cbd5e1' : '#64748b'}" stroke-width="${partner ? 1.4 : 1.6}"/>`);
  }

  // nodes
  for (const n of nodes) {
    const x = n.x + ox, y = n.y + oy;
    if (persons.has(n.id)) {
      const p = persons.get(n.id);
      const fill = p.sex === 'M' ? '#dbeafe' : p.sex === 'F' ? '#fce7f3' : '#e2e8f0';
      const edge = p.sex === 'M' ? '#2563eb' : p.sex === 'F' ? '#db2777' : '#64748b';
      out.push(`<rect x="${x.toFixed(1)}" y="${y.toFixed(1)}" width="${n.width}" height="${n.height}" rx="7" fill="${fill}" stroke="${edge}" stroke-width="1.3"/>`);
      out.push(`<text x="${(x + n.width / 2).toFixed(1)}" y="${(y + n.height / 2 + 1).toFixed(1)}" font-size="13" text-anchor="middle" dominant-baseline="middle" fill="#0f172a">${esc(p.name)}</text>`);
      out.push(`<text x="${(x + n.width / 2).toFixed(1)}" y="${(y + n.height - 5).toFixed(1)}" font-size="8.5" text-anchor="middle" fill="#64748b">${p.birthYear ?? ''}</text>`);
    } else {
      // union node: small dark dot
      const cx = x + n.width / 2, cy = y + n.height / 2;
      out.push(`<circle cx="${cx.toFixed(1)}" cy="${cy.toFixed(1)}" r="5" fill="#334155"/>`);
    }
  }

  out.push(`</svg>`);
  return out.join('\n');
}

module.exports = { toSvg, bbox };
