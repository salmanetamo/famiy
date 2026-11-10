import type { ElkNode } from 'elkjs/lib/elk-api';
import { describe, expect, it, vi } from 'vitest';
import { createWorkerEngine, type WorkerLike } from './workerEngine';
import type { LayoutRequestMessage, LayoutResponseMessage } from './workerProtocol';

function fakeWorker() {
  const posted: LayoutRequestMessage[] = [];
  const terminate = vi.fn();
  const worker: WorkerLike & { fail: () => void; badMessage: () => void } = {
    onmessage: null,
    onerror: null,
    onmessageerror: null,
    postMessage: (message) => {
      posted.push(message);
    },
    terminate,
    fail: () => worker.onerror?.(new ErrorEvent('error')),
    badMessage: () => worker.onmessageerror?.(new MessageEvent('messageerror')),
  };
  const reply = (message: LayoutResponseMessage) => {
    worker.onmessage?.({ data: message } as MessageEvent<LayoutResponseMessage>);
  };
  return { worker, posted, reply, terminate };
}

const graph = (id: string): ElkNode => ({ id });

describe('createWorkerEngine', () => {
  it('matches replies to requests by id, even when they arrive out of order', async () => {
    const { worker, posted, reply } = fakeWorker();
    const engine = createWorkerEngine(() => worker);
    const first = engine.layout(graph('a'));
    const second = engine.layout(graph('b'));
    expect(posted.map((m) => m.graph.id)).toEqual(['a', 'b']);
    reply({ id: posted[1]?.id ?? -1, result: graph('b-laid') });
    reply({ id: posted[0]?.id ?? -1, result: graph('a-laid') });
    expect((await first).id).toBe('a-laid');
    expect((await second).id).toBe('b-laid');
  });

  it('rejects a request the worker reports an error for, and leaves others alone', async () => {
    const { worker, posted, reply } = fakeWorker();
    const engine = createWorkerEngine(() => worker);
    const bad = engine.layout(graph('a'));
    const good = engine.layout(graph('b'));
    reply({ id: posted[0]?.id ?? -1, error: 'invalid graph' });
    reply({ id: posted[1]?.id ?? -1, result: graph('ok') });
    await expect(bad).rejects.toThrow('invalid graph');
    expect((await good).id).toBe('ok');
    expect(engine.isBroken()).toBe(false);
  });

  it('ignores a reply nobody is waiting for', () => {
    const { worker, reply } = fakeWorker();
    createWorkerEngine(() => worker);
    expect(() => {
      reply({ id: 999, result: graph('x') });
    }).not.toThrow();
  });

  it('rejects everything in flight when the worker crashes, and refuses new work', async () => {
    const { worker } = fakeWorker();
    const engine = createWorkerEngine(() => worker);
    const inFlight = engine.layout(graph('a'));
    worker.fail();
    await expect(inFlight).rejects.toThrow(/layout worker failed/);
    expect(engine.isBroken()).toBe(true);
    await expect(engine.layout(graph('b'))).rejects.toThrow(/not running/);
  });

  it('treats an unreadable message from the worker as a crash too', async () => {
    const { worker } = fakeWorker();
    const engine = createWorkerEngine(() => worker);
    const inFlight = engine.layout(graph('a'));
    worker.badMessage();
    await expect(inFlight).rejects.toThrow(/could not be read/);
    expect(engine.isBroken()).toBe(true);
  });

  it('rejects, instead of waiting forever, when the graph cannot be posted', async () => {
    const { worker } = fakeWorker();
    worker.postMessage = () => {
      throw new Error('could not be cloned');
    };
    const engine = createWorkerEngine(() => worker);
    await expect(engine.layout(graph('a'))).rejects.toThrow('could not be cloned');
    expect(engine.isBroken()).toBe(false);
  });

  it('terminate stops the worker and rejects what is in flight', async () => {
    const { worker, terminate } = fakeWorker();
    const engine = createWorkerEngine(() => worker);
    const inFlight = engine.layout(graph('a'));
    engine.terminate();
    await expect(inFlight).rejects.toThrow(/stopped/);
    expect(terminate).toHaveBeenCalledTimes(1);
    expect(engine.isBroken()).toBe(true);
  });
});

describe('getAppLayoutEngine', () => {
  it('shares one worker, and replaces it after it crashes', async () => {
    const created: { onerror: ((event: ErrorEvent) => void) | null; terminate: () => void }[] = [];
    class FakeWorker {
      onmessage = null;
      onerror: ((event: ErrorEvent) => void) | null = null;
      onmessageerror = null;
      postMessage = vi.fn();
      terminate = vi.fn();
      constructor() {
        created.push(this);
      }
    }
    vi.stubGlobal('Worker', FakeWorker);
    vi.resetModules();
    const { getAppLayoutEngine } = await import('./workerEngine');
    try {
      const first = getAppLayoutEngine();
      expect(getAppLayoutEngine()).toBe(first);
      expect(created).toHaveLength(1);

      created[0]?.onerror?.(new ErrorEvent('error')); // the worker crashes
      const replacement = getAppLayoutEngine();
      expect(replacement).not.toBe(first);
      expect(created).toHaveLength(2);
      expect(getAppLayoutEngine()).toBe(replacement);
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
