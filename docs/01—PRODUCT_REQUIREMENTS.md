# Product Requirements — Family Graph

## Vision
An interactive web application for exploring, visualizing, and managing family trees.

## Core features
- Person profiles with names, dates, photographs, biographies, and residence information.
- Family relationships with types and descriptions.
- Interactive graph with panning, zooming, node selection, and centering.
- Generational layout and age-based ordering where feasible.
- Search and navigation.
- Person and relationship management.
- Authentication, authorization, privacy, and data integrity.

## MVP scope
The MVP is a single working vertical slice. Essential vs. deferred:

| Essential (MVP) | Deferred (later phases) |
|-----------------|-------------------------|
| Person CRUD: name, birth/death dates (with uncertainty), one photo, short bio, residence | Media galleries, rich documents, sources/citations |
| Edges: `parent → child` and partnership, with type | Adoption / step / godparent as first-class nuance; emotional-closeness dimension |
| Siblings and genealogical distance **derived** via traversal | Precomputed / cached relationship metrics |
| Interactive graph: pan, zoom, select, center-on-person | True minimum-crossing optimization; timeline & map views |
| Generational (layered) layout, "good enough" ordering | GEDCOM import/export |
| Search by name | Fuzzy / phonetic search, full-text bios |
| Authentication; single owner per tree | Collaboration, sharing, granular privacy / ACLs |
| Handle incomplete data (unknown parents, approximate dates) | Performance work for 100k+ nodes |

Modelling note: the atomic edge is `parent → child`; partnership is a separate
edge type; siblings are derived (shared parent), never stored. Store relationship
*type*, compute genealogical *distance*, defer emotional *closeness*.

## Key constraints
- Family structures may contain multiple partnerships, blended families, and incomplete historical information.
- Distinguish relationship type, genealogical distance, and emotional closeness.
- Keep domain data separate from visual layout state.
- Model kinship as a DAG (a person has up to two parents), not a tree.
- Support an incremental MVP and evaluate performance at increasing graph sizes.
- Small scale: hundreds to low thousands of persons per tree, tens of thousands at most; read-heavy, write-light.
- Operate within ≤ $10/month on a single barebones VM, deployable and maintainable by a solo developer.

## Resolved questions
- **Single-user or collaborative?** → Single-owner trees for the MVP; the data
  model is designed so collaboration can be added later without a rewrite.
  Real-time multi-editor collaboration is deferred.
- **Initial target size of a family network?** → Small (see Key constraints).
  This assumption underpins ADR-001 (database) and ADR-002 (rendering); PoC-1
  validates it with evidence.
- **Required import/export formats?** → None in the MVP. GEDCOM is the known
  future format, deferred.
- **Preferred hosting budget?** → ≤ $10/month on a single VM (Hetzner leaning on
  price/performance; confirm provider at deploy time).

## Open questions
- **Privacy and data retention.** MVP posture: trees are private to their owner
  and gated by authentication; granular sharing / ACLs are deferred. Still to
  decide: handling of data about *living* relatives (PII), and any retention
  policy — revisit before the app holds real third-party data. Auth approach
  itself is decided in Phase 4.

## Definition of done
A deployed, tested, maintainable application with correct kinship (DAG) modelling
and enforced relationship invariants, an intuitive and performant graph interface,
and documented technical decisions (ADRs, PoC evidence, status).