import type { components } from './schema';

// Re-exports of the generated contract types, so the rest of the app never imports schema.d.ts.
export type User = components['schemas']['User'];
export type Credentials = components['schemas']['Credentials'];
export type ApiGraph = components['schemas']['Graph'];
export type PersonDetail = components['schemas']['Person'];
export type PersonSummary = components['schemas']['PersonSummary'];
