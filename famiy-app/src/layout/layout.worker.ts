// Runs ELK off the main thread (ADR-005 b): at 1,000 people layout took 2.4 to 3.2 seconds.
// This is the only file that imports the 1.6 MB elkjs bundle, so it stays out of the main chunk.
import ELK from 'elkjs/lib/elk.bundled.js';
import type { LayoutRequestMessage, LayoutResponseMessage } from './workerProtocol';

interface WorkerScope {
  onmessage: ((event: MessageEvent<LayoutRequestMessage>) => void) | null;
  postMessage(message: LayoutResponseMessage): void;
}

// The DOM lib types `self` as a Window; in a worker it is the worker scope. Only the two members
// declared above are used.
const scope = self as unknown as WorkerScope;
const elk = new ELK();

scope.onmessage = (event) => {
  const { id, graph } = event.data;
  elk.layout(graph).then(
    (result) => {
      scope.postMessage({ id, result });
    },
    (error: unknown) => {
      scope.postMessage({ id, error: error instanceof Error ? error.message : String(error) });
    },
  );
};
