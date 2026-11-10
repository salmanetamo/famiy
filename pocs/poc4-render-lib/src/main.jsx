import React, { useRef, useState, useEffect, memo } from 'react';
import { createRoot } from 'react-dom/client';
import { flushSync } from 'react-dom';
import { ReactFlow, ReactFlowProvider, useReactFlow, BaseEdge } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import cytoscape from 'cytoscape';

const params = new URLSearchParams(location.search);
const lib = params.get('lib');
const size = params.get('data');
const cull = params.get('cull') === '1';
const raf = () => new Promise((r) => requestAnimationFrame(() => r()));

// ---------- 1. custom React + SVG ----------
const Person = memo(({ n, sel }) => (
  <g transform={`translate(${n.x},${n.y})`} data-id={n.id} tabIndex={0} role="button">
    <rect width={n.w} height={n.h} rx="6" fill={sel ? '#fde68a' : '#fff'} stroke="#334" />
    <text x={n.w / 2} y={n.h / 2 + 4} textAnchor="middle" fontSize="12">{n.label}</text>
  </g>
));
function SvgApp({ layout, api }) {
  const [sel, setSel] = useState(null);
  const g = useRef(null);
  const view = useRef({ x: 0, y: 0, k: 1 });
  const apply = () => {
    g.current.style.transform = `translate(${view.current.x}px,${view.current.y}px) scale(${view.current.k})`;
  };
  useEffect(() => {
    api.setView = (x, y, k) => { view.current = { x, y, k }; apply(); };
    api.select = (id) => flushSync(() => setSel(id));
  });
  return (
    <svg width="100%" height="100%" style={{ position: 'absolute', inset: 0 }}>
      <g ref={g} style={{ transformOrigin: '0 0' }}>
        {layout.edges.map((e) => (
          <polyline key={e.id} points={e.pts.map((p) => p.join(',')).join(' ')} fill="none" stroke={e.kind === 'partner' ? '#a55' : '#678'} />
        ))}
        {layout.nodes.filter((n) => n.kind === 'person').map((n) => <Person key={n.id} n={n} sel={sel === n.id} />)}
        {layout.nodes.filter((n) => n.kind === 'union').map((n) => (
          <circle key={n.id} cx={n.x + n.w / 2} cy={n.y + n.h / 2} r="4" fill="#a55" />
        ))}
      </g>
    </svg>
  );
}

// ---------- 2. React Flow ----------
const PolyEdge = ({ id, data }) => <BaseEdge id={id} path={'M' + data.pts.map((p) => p.join(',')).join(' L')} />;
const edgeTypes = { poly: PolyEdge };
function RFInner({ layout, api }) {
  const rf = useReactFlow();
  const [nodes, setNodes] = useState(() =>
    layout.nodes.filter((n) => n.kind !== 'container').map((n) => ({
      id: n.id, position: { x: n.x, y: n.y }, data: { label: n.label || '' }, draggable: false, selectable: true,
      style: { width: n.w, height: n.h, fontSize: 12, padding: 0, borderRadius: n.kind === 'union' ? 7 : 6 },
    })));
  const edges = useRef(layout.edges.map((e) => ({ id: e.id, source: e.source, target: e.target, type: 'poly', data: { pts: e.pts } }))).current;
  useEffect(() => {
    api.setView = (x, y, k) => rf.setViewport({ x, y, zoom: k });
    api.select = (id) => flushSync(() => setNodes((ns) => ns.map((n) => ({ ...n, selected: n.id === id }))));
  });
  return (
    <ReactFlow nodes={nodes} edges={edges} edgeTypes={edgeTypes} nodesConnectable={false} elementsSelectable
      onlyRenderVisibleElements={cull} minZoom={0.05} proOptions={{ hideAttribution: true }} />
  );
}
const RFApp = (p) => (
  <ReactFlowProvider><div style={{ position: 'absolute', inset: 0 }}><RFInner {...p} /></div></ReactFlowProvider>
);

// ---------- 3. Cytoscape ----------
function CyApp({ layout, api }) {
  const ref = useRef(null);
  useEffect(() => {
    const els = [];
    for (const n of layout.nodes) {
      if (n.kind !== 'container') els.push({ data: { id: n.id, label: n.label || '' }, position: { x: n.x + n.w / 2, y: n.y + n.h / 2 }, classes: n.kind });
    }
    for (const e of layout.edges) els.push({ data: { id: e.id, source: e.source, target: e.target } });
    const cy = cytoscape({
      container: ref.current, elements: els, layout: { name: 'preset' },
      style: [
        { selector: 'node', style: { label: 'data(label)', shape: 'round-rectangle', width: 84, height: 40, 'font-size': 12, 'text-valign': 'center', 'background-color': '#fff', 'border-width': 1, 'border-color': '#334' } },
        { selector: '.union', style: { width: 8, height: 8, label: '' } },
        { selector: '.sel', style: { 'background-color': '#fde68a' } },
        { selector: 'edge', style: { 'curve-style': 'taxi', 'line-color': '#678', width: 1 } },
      ],
    });
    api.setView = (x, y, k) => { cy.viewport({ zoom: k, pan: { x, y } }); };
    api.select = (id) => { cy.elements().removeClass('sel'); cy.$id(id).addClass('sel'); };
    return () => cy.destroy();
  }, []);
  return <div ref={ref} style={{ position: 'absolute', inset: 0 }} />;
}

// ---------- harness ----------
const api = {};
window.runBench = async function () {
  const res = await fetch(`/layout-${size}.json`);
  const layout = await res.json();
  const root = createRoot(document.getElementById('root'));
  const App = { svg: SvgApp, rf: RFApp, cy: CyApp }[lib];
  const tr = performance.now();
  flushSync(() => root.render(<App layout={layout} api={api} />));
  await raf(); await raf();
  const renderMs = performance.now() - tr;
  const domEls = document.getElementsByTagName('*').length;
  await new Promise((r) => setTimeout(r, 300));
  const deltas = [];
  let last = performance.now();
  for (let i = 0; i < 90; i++) {
    const t = i / 89;
    api.setView(-t * 400, -t * 200, 1 - 0.5 * Math.sin(t * Math.PI) * 0.4);
    await raf();
    const now = performance.now(); deltas.push(now - last); last = now;
  }
  deltas.sort((a, b) => a - b);
  const med = deltas[45];
  const dropped = (deltas.filter((d) => d > 1000 / 30).length / deltas.length) * 100;
  const ids = layout.nodes.filter((n) => n.kind === 'person').map((n) => n.id);
  const sels = [];
  for (let i = 0; i < 5; i++) {
    const s = performance.now();
    api.select(ids[Math.floor(ids.length / 2) + i]);
    sels.push(performance.now() - s);
    await raf();
  }
  sels.sort((a, b) => a - b);
  return { lib, size, cull, persons: ids.length, domEls, renderMs: +renderMs.toFixed(1), medFps: +(1000 / med).toFixed(1), dropPct: +dropped.toFixed(0), selectMs: +sels[2].toFixed(1) };
};
