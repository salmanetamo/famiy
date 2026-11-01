# Data Model — Family Graph

_Last updated: 2026-11-01. Source of truth: `famiy-api/migrations/`. This page explains it;
if they disagree, the migrations win. Schema version: `000001_init`._

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
| `given_name` | text, not null | not blank (CHECK) |
| `family_name` | text, null | |
| `birth_date` | date, null | |
| `birth_approx` | boolean, not null, default false | models "approximate date"; the domain additionally requires `birth_date` when true |
| `death_date` | date, null | not before `birth_date` (CHECK) |
| `created_at` | timestamptz, not null, default now() | |

Not modelled yet: photo, bio, residence, owner/tree, sex. Each arrives as its own migration.

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
| **At most two parents per child** | `BEFORE INSERT` trigger `trg_max_two_parents` | → `too_many_parents` (409) |
| Partnership stored once, canonically | `CHECK (partner_a < partner_b)` + PK | planned |
| Death not before birth | `CHECK` | `person.New` → `validation_failed` |
| Given name not blank | `CHECK` | `person.New` → `validation_failed` |
| **Acyclic** (nobody is their own ancestor) | **not enforced** | **planned**, relationship domain layer |

### The two-parent trigger
A CHECK cannot count rows, so a trigger does. Two details differ from PoC-1:
1. It first **locks the child's `person` row (`FOR UPDATE`)**. Without the lock, concurrent
   inserts for one child each see "one parent" and both commit. This is covered by a
   concurrency test that fails without the lock and passes with it.
2. It **steps aside for an exact duplicate edge**, so the primary key reports "already
   exists" instead of the misleading "too many parents".

It fires on INSERT only; edges are inserted or deleted, never updated.

## Derived data (not stored)
- **Ancestors** — implemented: recursive CTE (`relationship/postgres.go`) with a `CYCLE`
  guard; `min(depth)` + `GROUP BY` collapses duplicate paths from diamonds such as cousin
  marriages. Each ancestor is returned once at its shortest generational distance.
- **Descendants, siblings, relationship path / LCA, ego network** — queries proven in PoC-1
  (`pocs/poc1-db-benchmark/sql/queries/`), **not yet implemented** in the API.

## Known gaps
- **Acyclicity** is guarded only defensively (the CTE's `CYCLE` clause stops runaway loops).
  A write-time check is needed before an endpoint can create parent links.
- No ownership/tenant column: single-owner trees depend on the pending authentication decision.
- Privacy of living relatives' data is an open product question (`01—PRODUCT_REQUIREMENTS.md`).
