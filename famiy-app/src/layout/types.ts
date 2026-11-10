import type { ElkNode } from 'elkjs/lib/elk-api';

export interface Point {
  x: number;
  y: number;
}

export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface PositionedPerson extends Rect {
  id: number;
}

/** The small dot between two partners that their children descend from. */
export interface PositionedUnion extends Rect {
  id: string;
  kind: string | null;
}

export interface RoutedEdge {
  id: string;
  /** `partner`: person to union. `descent`: union (or single parent) to child. */
  kind: 'partner' | 'descent';
  points: readonly Point[];
}

/** Everything the renderer needs: plain, serialisable coordinates and no domain logic. */
export interface Layout {
  persons: readonly PositionedPerson[];
  unions: readonly PositionedUnion[];
  edges: readonly RoutedEdge[];
  bounds: Rect;
}

/** Something that lays out an ELK graph: a Web Worker in the app, plain elkjs in tests. */
export interface LayoutEngine {
  layout(graph: ElkNode): Promise<ElkNode>;
}
