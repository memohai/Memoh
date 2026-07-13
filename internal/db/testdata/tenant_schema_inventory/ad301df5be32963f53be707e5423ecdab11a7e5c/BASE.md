# Tenant Schema Inventory — Pinned Upstream Base

- **`UPSTREAM_BASE_COMMIT`**: `ad301df5be32963f53be707e5423ecdab11a7e5c`
- **Branch base**: `origin/main` tip at implement time
- **Pinned (UTC+8) commit date**: 2026-07-13T13:43:22+08:00
- **Engine scope**: `postgresql-only` (post-PR #748 PostgreSQL single-engine baseline)

## Why this SHA

This is the immutable base commit for the Phase 1 upstream tenant-schema work
(`docs/superpowers/plans/2026-07-13-tenant-data-plane-cell-rls-phase-1-upstream-tenant-schema.md`
in the Memoh-Cloud repo). It was re-pinned at implement time from the live
`memohai/Memoh` `origin/main` tip, per the plan's "固定事实（实施时必须重钉）".

The plan's discovery-time reference `985a1d70dd5b7e6213cca0ce84220b56097fee85`
is now two commits behind this tip. The two intervening commits are:

- `ad301df5` feat(models): add optional model descriptions (#774)
- `985a1d70` fix(workspace): unify user-facing terminology and errors (#775)

`git diff --stat 985a1d70..ad301df5 -- db sqlc.yaml AGENTS.md` shows the only
schema-relevant change is `db/postgres/queries/models.sql` (5 insertions);
the engine layout, `sqlc.yaml`, and `[database]` scope are unchanged.

## Engine single-engine facts (verified at this SHA)

- `db/` contains only `postgres/` (plus `embed.go`); no `sqlite/` or `mysql/`.
- `sqlc.yaml` declares a single `engine: "postgresql"`.
- `AGENTS.md`: `[database]` — Database backend selection (`postgres`).
- Post-PR #748 ("refactor: remove sqlite and local desktop") PostgreSQL-only
  hosted model. **No second engine present.**

If any second engine is reintroduced upstream, STOP and escalate: a new
Accepted ADR (with a pinned SHA that actually contains that backend) is
required before adding any corresponding acceptance gate.

## Migration facts

- PostgreSQL canonical schema: `db/postgres/migrations/0001_init.up.sql`
  (48 `CREATE TABLE`, 47 `ON DELETE SET NULL` in canonical).
- PostgreSQL migration head at base: `0105_repair_superseded_message_visibility`.
- Next available incremental number: **`0106`** (numbering has a documented
  historical gap `0100 → 0102`, i.e. no `0101`; this does not affect next-number
  assignment).
- `ON DELETE SET NULL` occurrences across all migration files (up+down): 101.
- sqlc queries: 43 files under `db/postgres/queries/`.

## Upstream AGENTS migration rules (summary, authoritative at this SHA)

From `AGENTS.md` "Database, sqlc & Migrations":

1. `db/postgres/migrations/0001_init.up.sql` is the **canonical** full schema;
   every schema change must also update it to reflect the final state.
2. Incremental files (`0002_`, ...) contain only the upgrade diff.
3. Every incremental migration is **paired** (`.up.sql` + `.down.sql`).
4. Header comment on each file (migration name + brief description).
5. Idempotent DDL (`IF NOT EXISTS` / `IF EXISTS`).
6. `.down.sql` must fully reverse `.up.sql` in reverse order.
7. After creating/modifying migrations or queries, run `mise run sqlc-generate`
   and validate the PostgreSQL migration path.

## Cloud gitlink cross-reference (NOT a base)

The Memoh-Cloud fork snapshot gitlink is
`023cfc885f6cb02bb18c18a6c8a627a185ca92ef`. It is used **only** as a read-only
cross-reference for the SET NULL / table inventory and MUST NOT be used as
`UPSTREAM_BASE_COMMIT`.
