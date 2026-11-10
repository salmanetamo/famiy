import type { ElkNode } from 'elkjs/lib/elk-api';
import type { LayoutEngine } from './types';
import type { LayoutRequestMessage, LayoutResponseMessage } from './workerProtocol';

/** The part of a Web Worker this module uses, so tests can stand in a fake one. */
export interface WorkerLike {
  onmessage: ((event: MessageEvent<LayoutResponseMessage>) => void) | null;
  onerror: ((event: ErrorEvent) => void) | null;
  onmessageerror: ((event: MessageEvent) => void) | null;
  postMessage(message: LayoutRequestMessage): void;
  terminate(): void;
}

export interface WorkerLayoutEngine extends LayoutEngine {
  terminate(): void;
  /** True once the worker crashed or was stopped; such an engine must be replaced. */
  isBroken(): boolean;
}

interface Pending {
  resolve: (graph: ElkNode) => void;
  reject: (error: Error) => void;
}

const createLayoutWorker = (): WorkerLike =>
  new Worker(new URL('./layout.worker.ts', import.meta.url), { type: 'module' });

/** A LayoutEngine backed by a dedicated Web Worker. Requests may overlap; replies are matched by id. */
export function createWorkerEngine(
  createWorker: () => WorkerLike = createLayoutWorker,
): WorkerLayoutEngine {
  const worker = createWorker();
  const pending = new Map<number, Pending>();
  let nextId = 0;
  let broken = false;

  const failAll = (error: Error) => {
    for (const entry of pending.values()) entry.reject(error);
    pending.clear();
  };
  const crash = (message: string) => {
    broken = true;
    failAll(new Error(message));
  };

  worker.onmessage = (event) => {
    const message = event.data;
    const entry = pending.get(message.id);
    if (!entry) return;
    pending.delete(message.id);
    if ('error' in message) entry.reject(new Error(message.error));
    else entry.resolve(message.result);
  };
  worker.onerror = () => {
    crash('The layout worker failed.');
  };
  worker.onmessageerror = () => {
    crash('The layout worker sent a message that could not be read.');
  };

  return {
    layout(graph) {
      if (broken) return Promise.reject(new Error('The layout worker is not running.'));
      return new Promise<ElkNode>((resolve, reject) => {
        const id = ++nextId;
        pending.set(id, { resolve, reject });
        try {
          worker.postMessage({ id, graph });
        } catch (error) {
          // for example a graph that cannot be structured-cloned: do not leave the request waiting
          pending.delete(id);
          reject(error instanceof Error ? error : new Error(String(error)));
        }
      });
    },
    terminate() {
      worker.terminate();
      crash('The layout worker was stopped.');
    },
    isBroken: () => broken,
  };
}

let shared: WorkerLayoutEngine | undefined;

/** One worker for the whole app, created on first use and replaced if it ever crashes. */
export function getAppLayoutEngine(): LayoutEngine {
  if (shared?.isBroken() ?? false) {
    shared?.terminate();
    shared = undefined;
  }
  shared ??= createWorkerEngine();
  return shared;
}
