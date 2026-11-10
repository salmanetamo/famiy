import type { ElkNode } from 'elkjs/lib/elk-api';
import { describe, expect, it } from 'vitest';
import { buildFamilyGraph } from '../domain/family';
import { apiGraphFrom, edgeCases } from '../test/fixtures';
import { LayoutRunner } from './layoutRunner';
import type { LayoutEngine } from './types';

interface Call {
  resolve: (graph: ElkNode) => void;
  reject: (error: Error) => void;
  graph: ElkNode;
}

/** An engine whose answers the test controls: calls are queued until resolved or rejected. */
function controlledEngine() {
  const calls: Call[] = [];
  const engine: LayoutEngine = {
    layout: (graph) =>
      new Promise<ElkNode>((resolve, reject) => {
        calls.push({ resolve, reject, graph });
      }),
  };
  return { engine, calls };
}

/** What a real engine would return for this graph, positioned at the origin. */
const position = (node: ElkNode): ElkNode => ({
  ...node,
  x: 0,
  y: 0,
  ...(node.children ? { children: node.children.map(position) } : {}),
});
const placed = (graph: ElkNode): ElkNode => ({
  ...graph,
  children: (graph.children ?? []).map(position),
  edges: [],
});

const family = buildFamilyGraph(apiGraphFrom(edgeCases).api);

describe('LayoutRunner', () => {
  it('returns the layout of a request nobody overtook', async () => {
    const { engine, calls } = controlledEngine();
    const pending = new LayoutRunner(engine).request(family);
    calls[0]?.resolve(placed(calls[0].graph));
    expect((await pending)?.persons.length).toBeGreaterThan(0);
  });

  it('drops an old request that finishes after a newer one was made', async () => {
    const { engine, calls } = controlledEngine();
    const runner = new LayoutRunner(engine);
    const first = runner.request(family);
    const second = runner.request(family);
    // the newer one finishes first, then the older one limps in
    calls[1]?.resolve(placed(calls[1].graph));
    calls[0]?.resolve(placed(calls[0].graph));
    expect(await second).not.toBeNull();
    expect(await first).toBeNull();
  });

  it('drops an old request even when it finishes before the newer one', async () => {
    const { engine, calls } = controlledEngine();
    const runner = new LayoutRunner(engine);
    const first = runner.request(family);
    const second = runner.request(family);
    calls[0]?.resolve(placed(calls[0].graph));
    expect(await first).toBeNull();
    calls[1]?.resolve(placed(calls[1].graph));
    expect(await second).not.toBeNull();
  });

  it('surfaces an error from the latest request', async () => {
    const { engine, calls } = controlledEngine();
    const pending = new LayoutRunner(engine).request(family);
    calls[0]?.reject(new Error('worker crashed'));
    await expect(pending).rejects.toThrow('worker crashed');
  });

  it('swallows an error from a request that was already overtaken', async () => {
    const { engine, calls } = controlledEngine();
    const runner = new LayoutRunner(engine);
    const first = runner.request(family);
    const second = runner.request(family);
    calls[0]?.reject(new Error('old failure'));
    expect(await first).toBeNull();
    calls[1]?.resolve(placed(calls[1].graph));
    expect(await second).not.toBeNull();
  });
});
