import type { ElkNode } from 'elkjs/lib/elk-api';

/** Messages between the app and the layout worker: plain, structured-cloneable data only. */
export interface LayoutRequestMessage {
  id: number;
  graph: ElkNode;
}

export type LayoutResponseMessage = { id: number; result: ElkNode } | { id: number; error: string };
