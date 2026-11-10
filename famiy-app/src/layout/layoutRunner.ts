import type { FamilyGraph } from '../domain/family';
import { layoutFamily } from './layoutFamily';
import type { Layout, LayoutEngine } from './types';

/**
 * Runs layouts one request at a time from the caller's point of view: only the most recent
 * request's result counts. A request that was overtaken resolves to `null` (and its errors are
 * dropped), so a slow old layout can never replace the layout of a newer graph.
 */
export class LayoutRunner {
  private latest = 0;

  constructor(private readonly engine: LayoutEngine) {}

  async request(family: FamilyGraph): Promise<Layout | null> {
    const mine = ++this.latest;
    try {
      const layout = await layoutFamily(family, this.engine);
      return mine === this.latest ? layout : null;
    } catch (error) {
      if (mine !== this.latest) return null;
      throw error;
    }
  }
}
