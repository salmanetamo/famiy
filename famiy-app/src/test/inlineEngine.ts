import ELK from 'elkjs/lib/elk.bundled.js';
import type { LayoutEngine } from '../layout/types';

/** Real elkjs in the current thread, for tests (the app runs it in a Web Worker). */
export function createInlineEngine(): LayoutEngine {
  const elk = new ELK();
  return { layout: (graph) => elk.layout(graph) };
}
