# famiy-app (React + TypeScript frontend)

Run all commands from this folder. Node 22.13+ (`.nvmrc`, `engines`). Versions are pinned exactly
(`.npmrc` `save-exact`); verify against official sources before bumping. Decisions: ADR-002 and
ADR-005 in `docs/02—ARCHITECTURE_DECISIONS.md`. The API contract is `../famiy-api/api/openapi.yaml`.

## Commands

`npm run dev` (Vite on :5173) · `npm run check` (format, typecheck, lint, test, build) · `npm test`
· `npm run lint` · `npm run format` · `npm run api:types` (regenerate `src/api/schema.d.ts`; never edit
it by hand) · `npm run api:types:check` (CI: fails if the committed file is stale).

## Local development

1. Start the API (`cd ../famiy-api && make dev`; `.env` must have `REGISTRATION_ENABLED=true` and
   `COOKIE_SECURE=false` for plain-HTTP local use, which selects the unprefixed `famiy_session` cookie).
2. `npm run dev`. The browser only talks to `localhost:5173`; Vite proxies `/api/*` to the API
   (default `http://localhost:8080`, override with `API_PROXY_TARGET`) and strips the `/api` prefix.
3. Keep `changeOrigin: false` in `vite.config.ts`. The API's Origin check compares the Origin host
   with the request Host; rewriting Host makes every POST/PATCH/DELETE fail with `origin_not_allowed`
   (verified 2026-11-10). The API sends no CORS headers, so the proxy is required, not optional.

## Architecture rules (ADR-002, ADR-005)

- Domain state (people, edges) is separate from view state (coordinates, zoom, selection).
- Layout is a pure async function `layout(graph) -> Promise<Layout>` that runs in a Web Worker. It
  knows nothing about React or the DOM. The renderer (custom React + SVG) only draws coordinates.
- Server state goes through TanStack Query; after a mutation, refetch `/graph` (no delta endpoint).
- Only `src/api` calls `fetch`. Errors are typed with the server's `error.code`.
- Strict TypeScript: no `any`, no unexplained `as` or `!`.

## Testing

Vitest + Testing Library (jsdom). Layout and kinship logic are tested as pure functions with the
PoC-2 fixture in API shape. Playwright smoke tests come with the editing slice and run against a
throwaway database with one fixed account (registration is capped by `MAX_USERS` and rate-limited).

## Skills and agents

`.claude/skills/run-web-checks`. Agent: `web-reviewer`.
