---
name: new-migration
description: Create, apply and verify a golang-migrate up/down SQL migration pair in famiy-api. Use when the schema needs to change (new table, column, index, constraint, trigger).
---

# New migration

Run from `famiy-api/`. The dev database must be up (`make db-up`).

1. **Create the pair.** `make migrate-new name=add_person_bio` (snake_case, verb-first).
   This makes `migrations/NNNNNN_name.up.sql` and `.down.sql` with the next sequence number.
2. **Write `up`.** One logical change per migration. Enforce invariants in the database
   (CHECK, FK, UNIQUE, trigger), not only in Go. Comment *why* for anything non-obvious.
3. **Write `down`.** It must exactly undo `up`, in reverse dependency order
   (triggers/functions before tables). Use `IF EXISTS`.
4. **Never edit an applied migration** once it is merged or deployed. Add a new one.
   (Before the first merge, amending your own unmerged migration is acceptable; re-apply it
   with `make migrate-down` then `make migrate-up`.)
5. **Apply and inspect.** `make migrate-up`, then
   `docker exec famiy-db-1 psql -U famiy -d famiy -c '\d <table>'`.
6. **Prove the round trip.** `make migrate-down` then `make migrate-up`. Both must succeed
   and `down` must leave no leftover objects (`\dt`, `\df`).
7. **Tests.** Integration tests apply every `*.up.sql` into a throwaway database
   automatically (`internal/platform/testdb`). Add tests for any new constraint or trigger,
   including the failure path and the mapped domain error.
8. **Docs.** Update `docs/DATA_MODEL.md` (entities and invariants) and note the change in
   `docs/03—PROJECT_STATUS.md`. A new schema *decision* may need an ADR.

Report: the migration filenames, what you applied, and which of steps 5-7 you actually ran.
