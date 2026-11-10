---
name: run-web-checks
description: Run the full famiy-app quality gate (format, typecheck, lint, test, build, API types) and report failures. Use before committing, before opening a PR, or when asked whether the frontend is green.
---

# Run web checks

From `famiy-app/`. Node must satisfy `.nvmrc`/`engines` (22.13+). Run `npm ci` or `npm install` first if
`node_modules` is missing or `package.json` changed.

Run in this order and stop to diagnose at the first failure:

1. `npm run format:check` — fix with `npm run format`.
2. `npm run typecheck`
3. `npm run lint` — do not add `eslint-disable` without a reason comment.
4. `npm test`
5. `npm run build`
6. `npm run api:types:check` — regenerates `src/api/schema.d.ts` from `../famiy-api/api/openapi.yaml` and
   fails if it differs from git. It only proves anything when `schema.d.ts` is committed (an
   untracked file passes `git diff` vacuously), so say so if it is untracked.

`npm run check` runs 1–5 together.

Report honestly:
- Which steps passed, quoting the failing output of any that did not.
- Anything not run: end-to-end tests (need the API and a database), real-device performance,
  and CI itself (readable from GitHub only for pull requests).
- A test run that found no tests is not a pass.

Do not call the work green unless all steps ran and passed.
