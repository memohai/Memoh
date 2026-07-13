# Canonical vs Incremental Drift — Pinned Upstream Base

- **`UPSTREAM_BASE_COMMIT`**: `ad301df5be32963f53be707e5423ecdab11a7e5c`
- **Canonical**: `db/postgres/migrations/0001_init.up.sql` (48 `CREATE TABLE`, 1 VIEW,
  47 `ON DELETE SET NULL`, extension `pgcrypto`, enum `user_role`).
- **Incrementals**: `0002_*` .. `0105_*` (paired `.up.sql` + `.down.sql`; documented
  numbering gap `0100 → 0102`, no `0101`).

## Question this doc answers

The upstream `AGENTS.md` rule is that `0001_init.up.sql` is the **canonical full
schema** and every schema change must also update it to the final state. This doc
verifies that claim and enumerates every table that drifts between (a) what canonical
`0001_init.up.sql` declares and (b) what the full incremental chain
(`0001` applied, then `0002 … 0105` applied) would produce.

## Method

- Extracted every `CREATE TABLE` name from canonical (48).
- Grepped every `CREATE TABLE` / `DROP TABLE` (including dynamic `EXECUTE 'DROP …'`
  inside `DO $$` blocks) across all `*.up.sql` incrementals.
- Cross-checked which of those tables survive to the end of the incremental chain and
  whether they appear in canonical.
- Two drift directions exist:
  - **Forward drift**: an incremental `CREATE`s a table that canonical does NOT declare
    and no later incremental drops → present after a full incremental apply, absent from
    canonical (canonical is behind).
  - **Reverse drift**: canonical declares a table (or column) that a later incremental
    `DROP`s → present in canonical, absent after a full incremental apply (canonical is
    ahead / not re-synced to the drop).

## Verdict

Canonical `0001_init.up.sql` is **almost** the final state but has **three real drift
points**. It is NOT a byte-perfect projection of the incremental chain. A fresh install
runs the migrate driver over the embedded `*.sql` set (`db/embed.go` embeds
`postgres/migrations/*.sql`), so the migrate tool applies `0001` **and then** `0002…0105`
— it does not apply canonical alone. The practical final table set therefore = canonical
48, minus reverse-dropped `channel_identity_bind_codes`, plus forward-added
`media_assets` and `tasks` = **49 tables** on a fresh full-apply.

## Drift table 1 — `media_assets` (FORWARD drift)

- **Created**: `0007_media_assets.up.sql` (`CREATE TABLE IF NOT EXISTS media_assets`).
- **Dropped**: never (no `DROP TABLE media_assets` in any `.up.sql`).
- **In canonical `0001_init.up.sql`?** **NO.**
- **Disposition**: **present after full incremental apply, absent from canonical.**
  This is a genuine canonical omission — canonical was not updated to carry
  `media_assets`. It carries a `storage_provider_id UUID REFERENCES storage_providers(id)
  ON DELETE SET NULL` (the 5th of the 7 historical-extra SET NULL FKs; see
  `set_null_fks.json` historical section). Contract §14 marks it **待确认**; contract §18
  open-question #1 flags its final keep/tenantize decision. **Recommendation: confirm with
  owner** — if kept, canonical must be re-synced to add it and it becomes a
  `normal tenant table`.

## Drift table 2 — `tasks` (FORWARD drift)

- **Created**: `0063_add_task_tracking.up.sql` (`CREATE TABLE IF NOT EXISTS tasks`,
  `VARCHAR(255)` PK, `bot_id VARCHAR(255)` with **no FK constraint**).
- **Dropped**: never.
- **In canonical `0001_init.up.sql`?** **NO.**
- **Disposition**: **present after full incremental apply, absent from canonical.**
  Same class of drift as `media_assets`. Note it uses `VARCHAR(255)` PK/`bot_id` (not
  UUID) and has **no** DB-level FK to `bots`, so it is a soft-referenced side table.
  Contract §14 marks it **待确认**. **Recommendation: confirm with owner**; if kept, it
  becomes a `normal tenant table` (PK → `(tenant_id, id)`; the soft `bot_id` reference
  needs same-tenant application validation per §8.5).

## Drift table 4 — `tts_providers` / `tts_models` (PATH-DEPENDENT drift) — CORRECTED

**Correction (verified against a live PostgreSQL 18 full apply):** an earlier
revision of this doc listed `tts_providers` / `tts_models` as "fully-historical
(dropped by 0061)". That is **wrong for a fresh install**. The truth is
path-dependent:

- `0029_tts_provider.up.sql` creates both tables (`CREATE TABLE IF NOT EXISTS`,
  UUID `id` PK).
- `0061_unify_providers.up.sql` migrates + drops them, **but its whole DO block
  early-returns** (`IF NOT EXISTS (… 'llm_providers') THEN RETURN`). On a fresh
  DB, `0001_init` already ships the unified `providers` schema and there is no
  `llm_providers`, so `0061` does **nothing** and the tts DROP never runs.

Result:
- **Fresh install (0001→0105):** `tts_providers` + `tts_models` **survive** →
  present in the applied final state.
- **Legacy upgrade (had `llm_providers`):** `0061` runs fully and **drops** them.

Both tables therefore are **normal tenant tables on the fresh/greenfield path**
and must be tenantized. Because Cloud/self-hosted greenfield installs run the
fresh path, they are in scope. Tenantization enumerates tables from the applied
`information_schema`, so it naturally picks them up on whichever path the target
DB actually took.

## Drift table 3 — `channel_identity_bind_codes` (REVERSE drift)

- **Created**: canonical `0001_init.up.sql` (line ~414) AND
  `0001_init` is where it lives — it is one of the canonical 48.
- **Dropped**: `0080_remove_channel_identity_binding.up.sql`
  (`DROP TABLE IF EXISTS channel_identity_bind_codes;`). The same migration also
  `DROP COLUMN`s `channel_identities.user_id` and `DROP INDEX
  idx_channel_identities_user_id`.
- **In canonical `0001_init.up.sql`?** **YES** (both the table and
  `channel_identities.user_id`).
- **Disposition**: **present in canonical, absent after full incremental apply.**
  This is the reverse of the other two: canonical is **ahead** of the drop — it still
  declares a table and column that the incremental chain removes. On a fresh full-apply
  the table/column are created by `0001` then dropped by `0080`, so they do not survive.
  Contract §14 lists `channel_identity_bind_codes` as a tenant table (it only saw
  canonical), but the drift means it should likely NOT be tenantized. Classified
  **待确认** in `classification.md` / `tables.json`. **This also affects the SET NULL
  inventory**: canonical `channel_identities.user_id → users ON DELETE SET NULL` is row #1
  of the oracle's 47, yet that column is dropped by 0080. It is retained in the canonical
  SET NULL section (it must, to match the oracle) but flagged here as
  reverse-drift-affected.

## Fully-historical tables (created AND dropped in incrementals; never in canonical)

These are neither in canonical nor in the final full-apply state — pure historical
churn, listed for completeness and because several carry historical-extra SET NULL FKs
(contract §8.3):

| Table | Created (up) | Dropped (up) | Historical SET NULL it carried |
|---|---|---|---|
| `browser_contexts` | `0027_browser_contexts.up.sql` | `0078_drop_browser_gateway.up.sql` | (parent of) `bots.browser_context_id → browser_contexts` |
| `bot_inbox` | `0011_add_inbox.up.sql` | `0039_drop_inbox.up.sql` | — |
| `subagents` | (pre-0043; created earlier) | `0043_drop_subagents_add_parent_session.up.sql` | — |
| `bot_members` | (earlier) | `0031_chat_acl_remove_bot_members.up.sql` | — |
| `bot_preauth_keys` | (earlier) | `0031_chat_acl_remove_bot_members.up.sql` | `bot_preauth_keys.issued_by_user_id → users` (0031 down) |
| `email_provider_owner_map` | (transient in 0100) | `0100_email_provider_user_scope.up.sql` | — |
| `_memoh_history_turn_backfill` | `0103` (temp backfill) | `0103_message_turn_read_model.up.sql` | — (transient scratch table) |

## SET NULL cross-check

- Canonical `0001_init.up.sql`: **47** `ON DELETE SET NULL` FKs — parse verified to
  **exactly equal** the pinned `.setnull_canonical_oracle.json` (47 rows, same
  child/column/parent triples, same order). Two of the 47 come from deferred
  `ALTER TABLE … ADD CONSTRAINT … FOREIGN KEY … ON DELETE SET NULL` statements
  (`bot_channel_routes.active_session_id → bot_sessions` and
  `bot_history_messages.compact_id → bot_history_message_compacts`); the oracle's
  line-parser rendered these two with `column = "FOREIGN"` and `column = "ALTER"`
  respectively, and `set_null_fks.json` reproduces them verbatim to match.
- Full up/down history adds **7** distinct historical-extra SET NULL variants NOT in
  canonical (contract §8.3: 47 + 7 = **54** distinct child/column/parent combos):
  1. `bot_history_messages.route_id → bot_channel_routes` (`0036` down)
  2. `bot_preauth_keys.issued_by_user_id → users` (`0031` down)
  3. `bots.browser_context_id → browser_contexts` (`0027` up / `0078` down)
  4. `bots.embedding_model_id → models` (`0020` down)
  5. `bots.memory_model_id → models` (`0020` down)
  6. `bots.tts_model_id → tts_models` (`0029` up / `0061` down; final target is now
     `models`, which IS in the canonical 47)
  7. `media_assets.storage_provider_id → storage_providers` (`0007` up; canonical drift)

  These match contract §8.3's enumerated 7 exactly. See `set_null_fks.json` `historical`
  section (each `migrated:false`).

## Summary

| Metric | Value |
|---|---|
| Canonical `CREATE TABLE` | 48 |
| Canonical VIEW | 1 (`bot_visible_history_messages`) |
| Canonical `ON DELETE SET NULL` | 47 (== oracle) |
| Historical-extra SET NULL (not in canonical) | 7 |
| Total distinct SET NULL across history | 54 |
| Forward-drift tables (in fresh full-apply, not in canonical) | 4 — `media_assets`, `tasks`, `tts_providers`, `tts_models` |
| Reverse-drift tables (in canonical, dropped by incremental) | 1 — `channel_identity_bind_codes` |
| Fully-historical tables (never survive; dropped in history) | `browser_contexts`, `bot_inbox`, `subagents`, `bot_members`, `bot_preauth_keys`, `email_provider_owner_map`, `_memoh_history_turn_backfill` (transient) |
| Effective tenant tables on a fresh full-apply | **51** (verified via `information_schema`: 53 base tables − `schema_migrations` tooling − `tenants` root); tts pair survives because `0061` early-returns on fresh DBs |

**Action for implementers**: the source of truth is the **applied schema**, not
canonical text (see `BASE.md` migration policy — existing migrations are frozen;
we do NOT re-sync `0001_init`). Tenantization enumerates tenant tables from the
live `information_schema` after applying `0001→0107`, so it targets the true
applied state on whichever path (fresh vs legacy) the DB took. Do not rely on the
canonical text or on static drop-scans (which miss guarded/early-return blocks
like `0061`).
