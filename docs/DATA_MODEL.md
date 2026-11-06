# Data Model — Family Graph

_Last updated: 2026-11-02. Source of truth: `famiy-api/migrations/`. This page explains it;
if they disagree, the migrations win. Schema version: `000005_partnership_update_guard`._

## Principles (ADR-001)
- Kinship is a **DAG**, not a tree: a person has up to two parents.
- The atomic edge is **`parent → child`**. **Partnership** is a separate, undirected edge.
- **Siblings and genealogical distance are derived** by traversal, never stored.
- Domain data is kept separate from layout/view state (coordinates never enter this schema).

## Entities
### `person`
| Column | Type | Notes |
|---|---|---|
| `id` | bigint identity, PK | |
| `owner_user_id` | bigint, not null, FK `app_user` `ON DELETE RESTRICT` | the account that owns the person; **immutable** (trigger); an account that owns people cannot be deleted |
| `given_name` | text, not null | not blank, ≤ 200 characters (CHECK) |
| `family_name` | text, null | ≤ 200 characters (CHECK) |
| `birth_date` | date, null | ≥ 1000-01-01 (CHECK); the domain also rejects future dates |
| `birth_approx` | boolean, not null, default false | models "approximate date"; the domain additionally requires `birth_date` when true |
| `death_date` | date, null | ≥ 1000-01-01 and not before `birth_date` (CHECK); the domain also rejects future dates |
| `created_at` | timestamptz, not null, default now() | |

Not modelled yet: photo, bio, residence, owner/tree, sex. Each arrives as its own migration.

### `app_user` and `session` (ADR-004)
`app_user(id, email, password_hash, created_at)` — `email` is stored trimmed and lower-cased
(CHECK keeps it so) with a UNIQUE constraint, giving case-insensitive uniqueness;
`password_hash` is a PHC-format argon2id string. `session(token_hash, user_id, created_at,
expires_at)` — PK is the **SHA-256 of the session token** (exactly 32 bytes, CHECK), so the
database never holds a usable token; rows cascade-delete with their user and expired rows
are ignored on lookup and purged on login. Account creation is serialised by an advisory
lock so `MAX_USERS` cannot be exceeded under concurrent signups.
Every `person` belongs to exactly one `app_user` (`owner_user_id`). The API scopes every
read and write to the signed-in account and reports another account's person as 404, the same
as a missing one.

### `parent_child` (the atomic kinship edge)
`(parent_id, child_id)` — composite PK, both FKs to `person` with `ON DELETE CASCADE`.
Indexes: `idx_pc_child` (walk up), `idx_pc_parent` (walk down; technically redundant with
the PK's leading column, kept to make intent explicit).

### `partnership`
`(partner_a, partner_b, kind)` — composite PK, FKs to `person` with `ON DELETE CASCADE`,
`kind` defaults to `marriage`. Index `idx_partnership_b` for lookups from the `b` side.
Schema exists; **no API yet**.

## Invariants and where they are enforced
| Invariant | Database | Domain / API |
|---|---|---|
| No self-parent | `CHECK (parent_id <> child_id)` | `ParentLink.Validate` → `self_parent` |
| No duplicate edge | PK `(parent_id, child_id)` | → `duplicate_parent` (409) |
| Both ends exist | FKs | → `person_not_found` |
| **At most two parents per child** | `BEFORE INSERT` trigger `trg_parent_link_rules` (constraint name `max_two_parents`) | → `too_many_parents` (409) |
| **Acyclic** (nobody is their own ancestor) | same trigger (constraint name `no_cycle`) | → `would_create_cycle` (409) |
| **Both ends of a link have the same owner** | same trigger (`same_owner`); `trg_partnership_same_owner` for partnerships (fires on INSERT and on UPDATE of either partner) | service checks both persons are the caller's → `person_not_found` (404); a DB-level hit is also reported as `person_not_found` so existence never leaks |
| At most `MAX_PERSONS_PER_ACCOUNT` people per account (default 10000) | none (a count cannot be a CHECK) | `Insert` takes a per-owner advisory lock, counts, inserts → `person_limit_reached` (409). Bounds the `/graph` payload |
| Ownership never changes | `trg_person_owner_immutable` (`owner_immutable`) | n/a |
| Partnership stored once, canonically | `CHECK (partner_a < partner_b)` + PK | planned |
| Death not before birth | `CHECK` | `person.New` → `validation_failed` |
| Given name not blank / names ≤ 200 chars / dates ≥ 1000-01-01 | `CHECK` | `person.New` → `validation_failed` |

### The parent-link trigger (`enforce_parent_link_rules`)
A CHECK cannot count rows or walk a graph, so one `BEFORE INSERT` trigger enforces the two
rules that depend on other rows. Differences from PoC-1's version, all covered by tests:
1. **It serializes link inserts with a transaction-scoped advisory lock.** Without it,
   concurrent inserts each pass the check and together break the rule. A per-child row lock
   is not enough: `A→B` and `B→A` lock *different* children, so both would see no cycle.
2. **The advisory lock is taken before any row lock.** An earlier draft took a per-child
   `FOR UPDATE` first; opposing inserts then **deadlocked** (each held a person row the other
   needed for its foreign-key check while waiting on the advisory lock). Keep this order.
3. **It steps aside for an exact duplicate edge and for a self-link**, so the primary key and
   the `no_self_parent` CHECK report "already exists" / "self parent" instead of a misleading
   "too many parents".
4. **Each rule raises with a `CONSTRAINT` name** (`same_owner`, `max_two_parents`,
   `no_cycle`), checked in that order, and the
   application maps on that name, never on message text.

The concurrency tests (two-parent limit across 15 children; 25 opposing `A→B`/`B→A` pairs)
fail 3 runs out of 3 with the advisory lock removed and pass 3 out of 3 with it.

Caveats: the check relies on READ COMMITTED (the default), so do not insert links under
REPEATABLE READ. Writes to `parent_child` are globally serialized, which is fine for a
write-light app; if links become per-tree, key the lock by tree. The trigger fires on INSERT
only: edges are inserted or deleted, never updated (a table comment says so).

## Derived data (not stored)
- **Ancestors** — implemented: recursive CTE (`relationship/postgres.go`) with a `CYCLE`
  guard; `min(depth)` + `GROUP BY` collapses duplicate paths from diamonds such as cousin
  marriages. Each ancestor is returned once at its shortest generational distance.
- **The whole-account graph** — implemented (`internal/graph`): persons, links and partnerships read in one `REPEATABLE READ, READ ONLY` transaction. An edge's owner is the owner of its parent / `partner_a` person (the `same_owner` triggers make the other end match).
- **Descendants, siblings, relationship path / LCA, ego network** — queries proven in PoC-1
  (`pocs/poc1-db-benchmark/sql/queries/`), **not yet implemented** in the API.

## Known gaps
- Acyclicity is enforced on insert. Deleting an edge cannot create a cycle, so no check is
  needed there; the CTE keeps its `CYCLE` clause as defence in depth.
- Ownership is per account (`owner_user_id`), not per tree. When collaboration or trees arrive
  this becomes a `tree` / `tree_member` model and persons are re-keyed (ADR-004 Consequences).
- `000004` adds a `NOT NULL` column and fails if `person` already has rows. Once real data
  exists, any similar change needs a backfill migration.
- Privacy of living relatives' data is an open product question (`01—PRODUCT_REQUIREMENTS.md`).
