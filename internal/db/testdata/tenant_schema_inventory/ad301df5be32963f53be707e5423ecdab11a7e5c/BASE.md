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

- PostgreSQL migration head at base: `0105_repair_superseded_message_visibility`.
- Next available incremental number: **`0106`** (numbering has a documented
  historical gap `0100 → 0102`, i.e. no `0101`; this does not affect next-number
  assignment).
- Historical file `db/postgres/migrations/0001_init.up.sql` has 48 `CREATE TABLE`
  and 47 `ON DELETE SET NULL`, but it is a **historical snapshot, not the final
  schema** (see policy below and `canonical_vs_incremental_diff.md`).
- `ON DELETE SET NULL` occurrences across all migration files (up+down): 101.
- sqlc queries: 43 files under `db/postgres/queries/`.

## MIGRATION POLICY FOR THIS WORK (overrides upstream AGENTS)

**This tenant-schema work does NOT modify any existing migration file** (`0001_init`
through `0105`). Every change lands as a **new incremental** (`0106+`), paired
`.up.sql`/`.down.sql`, idempotent DDL, applied on top of the existing chain.

We deliberately **reject** the upstream `AGENTS.md` rule that treats
`0001_init.up.sql` as the canonical full schema that must be back-filled to the
final state. That rule is a mistaken invariant: at this base it is already
violated (canonical omits `media_assets` and `tasks`, and still declares
`channel_identity_bind_codes` which incremental `0080` drops). Editing frozen
migrations to "re-sync canonical" would rewrite applied history and is forbidden
here.

**Source of truth for the final schema = the actual state produced by applying
`0001 → 0105` (then our `0106+`) on an empty database**, verified against a real
PostgreSQL 18, NOT the text of `0001_init.up.sql`. The `tables.json` /
`set_null_fks.json` inventory in this directory records the canonical text for
reference, but downstream tenantization targets the applied final state (49
tables at base: canonical 48 − `channel_identity_bind_codes` + `media_assets`
+ `tasks`).

### Open PR-review question: canonical-authority reconciliation

An independent review noted that "updating `0001_init` to the final state" is
NOT the same as "rewriting applied incremental history" — the upstream rule only
asks that `0001_init` (the canonical file) reflect the final state, which does
not require touching `0002`–`0105`. So there are two defensible ways to land this
upstream, to be decided at PR time:

1. **Keep `0001_init` frozen, revise the AGENTS rule** (this doc's current
   stance): declare applied-state the source of truth and note the canonical
   invariant is already violated at base. Smaller diff; changes upstream policy.
2. **Back-fill `0001_init` to the tenant-ized final state** (satisfy AGENTS as
   written): edit only `0001_init.up.sql` (a fresh-install file), leaving the
   incrementals `0002`–`0105` untouched. Our `0106+` are idempotent
   (`IF NOT EXISTS`), so they no-op on a fresh install and still upgrade legacy
   installs. Larger, error-prone diff; keeps upstream policy intact.

Either is acceptable; the choice is a project/PR decision, not a correctness one.

### Note: SET NULL clear-refs are repository-level, not CTE

Empirically verified on PostgreSQL 18: a single-statement CTE that clears a child
column and then deletes the parent does NOT satisfy the FK — the DELETE's
`ON DELETE RESTRICT` check does not observe a same-statement data-modifying CTE's
UPDATE, so it still errors. The schema contract's requirement ("repository clears
the nullable ref column in the SAME TRANSACTION, then deletes the parent") is
therefore implemented as SEPARATE statements inside a transaction
(`internal/db/postgres/store/clear_refs.go`), which is verified to work.

## Upstream AGENTS migration rules (as written at this SHA — for reference)

From `AGENTS.md` "Database, sqlc & Migrations" (item 1 is overridden above):

1. ~~`db/postgres/migrations/0001_init.up.sql` is the canonical full schema;
   every schema change must also update it to reflect the final state.~~
   **OVERRIDDEN — see policy above; we do not edit frozen migrations.**
2. Incremental files (`0002_`, ...) contain only the upgrade diff. **(kept)**
3. Every incremental migration is **paired** (`.up.sql` + `.down.sql`). **(kept)**
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
