import type { ElkNode } from 'elkjs/lib/elk-api';
import type { FamilyGraph } from '../domain/family';
import { buildElkGraph, edgeKindOf } from './elkGraph';
import type {
  Layout,
  LayoutEngine,
  Point,
  PositionedPerson,
  PositionedUnion,
  Rect,
  RoutedEdge,
} from './types';

const EMPTY_LAYOUT: Layout = {
  persons: [],
  unions: [],
  edges: [],
  bounds: { x: 0, y: 0, width: 0, height: 0 },
};

function boundsOf(rects: readonly Rect[], points: readonly Point[]): Rect {
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const r of rects) {
    minX = Math.min(minX, r.x);
    minY = Math.min(minY, r.y);
    maxX = Math.max(maxX, r.x + r.width);
    maxY = Math.max(maxY, r.y + r.height);
  }
  for (const p of points) {
    minX = Math.min(minX, p.x);
    minY = Math.min(minY, p.y);
    maxX = Math.max(maxX, p.x);
    maxY = Math.max(maxY, p.y);
  }
  return { x: minX, y: minY, width: maxX - minX, height: maxY - minY };
}

/**
 * Turns ELK's result into plain coordinates. ELK reports a child's position relative to its
 * container, so offsets are accumulated down the tree; containers themselves are not drawn.
 */
export function toLayout(laid: ElkNode, kindOfUnion: ReadonlyMap<string, string | null>): Layout {
  const persons: PositionedPerson[] = [];
  const unions: PositionedUnion[] = [];
  const edges: RoutedEdge[] = [];

  const walk = (node: ElkNode, offsetX: number, offsetY: number) => {
    for (const edge of node.edges ?? []) {
      const section = edge.sections?.[0];
      if (!section) continue;
      const points = [section.startPoint, ...(section.bendPoints ?? []), section.endPoint].map(
        (p) => ({ x: p.x + offsetX, y: p.y + offsetY }),
      );
      edges.push({ id: edge.id, kind: edgeKindOf(edge.id), points });
    }
    for (const child of node.children ?? []) {
      if (child.x === undefined || child.y === undefined) {
        throw new Error(`layout returned no position for node ${child.id}`);
      }
      const rect = {
        x: offsetX + child.x,
        y: offsetY + child.y,
        width: child.width ?? 0,
        height: child.height ?? 0,
      };
      if (child.id.startsWith('p')) {
        persons.push({ id: Number(child.id.slice(1)), ...rect });
      } else if (child.id.startsWith('u')) {
        const id = child.id.slice(1);
        unions.push({ id, kind: kindOfUnion.get(id) ?? null, ...rect });
      } else {
        walk(child, rect.x, rect.y);
      }
    }
  };
  walk(laid, 0, 0);

  const bounds = boundsOf(
    [...persons, ...unions],
    edges.flatMap((e) => e.points),
  );
  return { persons, unions, edges, bounds };
}

/**
 * The layout service (ADR-002): domain data in, coordinates out. It knows nothing about React or
 * the DOM, and the engine that runs ELK is injected (a Web Worker in the app).
 */
export async function layoutFamily(family: FamilyGraph, engine: LayoutEngine): Promise<Layout> {
  if (family.persons.length === 0) return EMPTY_LAYOUT;
  const laid = await engine.layout(buildElkGraph(family));
  return toLayout(laid, new Map(family.unions.map((u) => [u.id, u.kind])));
}
