# Family Graph (famiy)

Web app for exploring and managing family trees. Kinship is a DAG (up to two
parents per person), not a tree. Single-owner trees for the MVP. Small scale
(hundreds to low thousands of persons per tree); one ≤ $10/mo VM; solo-maintained.

## Source of truth
Read `docs/` before changing architecture: `01—PRODUCT_REQUIREMENTS.md`,
`02—ARCHITECTURE_DECISIONS.md` (ADRs + PoC evidence), `03—PROJECT_STATUS.md`.
Do not relitigate accepted ADRs; propose a new/updated ADR instead.

## Locked stack (see ADRs, not duplicated here)
- Backend: Go REST API, PostgreSQL only, recursive CTEs for traversal (ADR-001, 003).
- Frontend (later): React + TypeScript; elkjs layout, SVG rendering (ADR-002).
- Modular monolith: `person`, `relationship`, `platform`. Keep it simple.

## Repo layout
- `famiy-api/`  Go backend (own go.mod). See `famiy-api/CLAUDE.md`.
- `famiy-app/`  frontend (not created yet).
- `deploy/`     docker-compose, Caddyfile.
- `docs/`       requirements, ADRs, status, architecture docs.
- `pocs/`       finished proofs of concept; reference only, do not extend.
- `.claude/`    project skills and agents.

## Conventions
- Always say whether something is proposed, implemented, tested, or deployed.
- Verify current library/tool versions against official sources before pinning.
- When a decision changes, update the ADRs and `03—PROJECT_STATUS.md` in the same change.
- Branches: `feat/…`, `fix/…`, `docs/…`, `chore/…`. Commits: Conventional Commits
  (`feat: …`). Small, incremental commits; no direct pushes of broken builds.
- No secrets in git. Config via environment; `.env.example` documents it.
