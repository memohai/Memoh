# Canonical Table Classification — Pinned Upstream Base

- **`UPSTREAM_BASE_COMMIT`**: `ad301df5be32963f53be707e5423ecdab11a7e5c`
- **Canonical source**: `db/postgres/migrations/0001_init.up.sql` (48 `CREATE TABLE`, 1 VIEW)
- **Contract**: `docs/superpowers/specs/tenant-schema-contract.md` §5 (global allowlist),
  §8 (SET NULL inventory), §14 (classification matrix) — read-only reference in the
  Memoh-Cloud repo.
- **Method**: every canonical `CREATE TABLE` classified per contract §14. Default =
  `normal tenant table` (gets `tenant_id`). Two NEW meta-tables (`tenants`,
  `tenant_write_fences`) do not exist yet and are noted as to-be-created. Drift tables
  cross-checked against the full incremental history (see
  `canonical_vs_incremental_diff.md`).

## Classification buckets (contract §14)

| Bucket | Meaning |
|---|---|
| `tenants` (root, NEW) | Root entity; `id` IS the `tenant_id`; no extra `tenant_id` column; must FORCE RLS `id = current tenant`. **Does not exist in canonical — to be created.** |
| `tenant_write_fences` (security meta, NEW) | Global security meta-table; `tenant_id` PK/FK → `tenants(id)`; **no RLS**; Runtime/PUBLIC have no direct table ACL. **Does not exist in canonical — to be created.** |
| `normal tenant table` | Default. Business table; gets `tenant_id NOT NULL`, composite PK/UK/FK, RLS. |
| `global allowlist tool table` | Only `schema_migrations`-style tooling metadata. **Per contract §5.1/§5.2 there are NO business global tables.** |
| `待确认` (to-confirm) | Drift / insufficient evidence; disposition note required. |

## NEW tables (to be created — NOT part of the canonical 48)

| Table | Bucket | Disposition |
|---|---|---|
| `tenants` | root special-case (NEW) | Create per §4.1: `PRIMARY KEY (id)`, no `tenant_id`, FORCE RLS `id = current tenant`. Seed one `DEFAULT_TENANT_ID` singleton row for self-hosted. |
| `tenant_write_fences` | no-RLS security meta allowlist (NEW) | Create per §5.3: `tenant_id` PK + FK → `tenants(id)` `ON DELETE RESTRICT`, `fencing_token BIGINT CHECK (>0)`, `write_enabled BOOLEAN`, `updated_at`; RLS/FORCE RLS both off; monotonic trigger + CAS fn + 2 named runtime helpers; Runtime/PUBLIC no table ACL. |

## Tooling metadata (not a canonical business table)

| Object | Bucket | Note |
|---|---|---|
| `schema_migrations` | global allowlist tool table (tooling metadata) | golang-migrate OSS-stream version table. NOT present in `0001_init.up.sql` (managed by the migrate driver, embedded via `db/embed.go`). Not tenant-exported, not a readable/writable global business table (§5.1.4). Listed here for completeness; it is not one of the canonical 48. |

> Note: no `cloud_*` tables exist in this upstream canonical (they live only in the
> Cloud `memoh-migrate` stream, out of scope for this OSS inventory).

## Canonical 48 tables — classification

All 48 canonical business tables classify as **`normal tenant table`** (default: gets
`tenant_id`) EXCEPT `channel_identity_bind_codes`, which is **`待确认`** (reverse drift).
Per contract §5.1/§5.2 and §14, `providers`, `users`, and every other install-level
config table are explicitly tenantized — there is **no business global-table allowlist**.

| # | Table | Domain (§14) | Classification | Note |
|---|---|---|---|---|
| 1 | `users` | account | normal tenant table | UK → `(tenant_id, email)`/`(tenant_id, username)` |
| 2 | `channel_identities` | channel identity | normal tenant table | UK → `(tenant_id, channel_type, channel_subject_id)`. NB: canonical `user_id` FK is dropped by 0080 (see diff doc). |
| 3 | `user_channel_bindings` | channel identity | normal tenant table | |
| 4 | `user_channel_identity_bindings` | channel identity | normal tenant table | |
| 5 | `channel_link_codes` | channel identity | normal tenant table | PK → `(tenant_id, token)` |
| 6 | `channel_identity_bind_codes` | channel identity | **待确认** | In canonical 0001 but **DROPPED by incremental `0080_remove_channel_identity_binding.up.sql`**. Absent from final full-apply state. Disposition: confirm keep-vs-drop before tenantizing; contract §14 lists it as a tenant table but the drift means it may not survive. Default lean: treat as dropped (do NOT tenantize) pending owner confirmation. |
| 7 | `providers` | provider/model | normal tenant table | name-unique → tenant-internal; NOT a global allowlist table (§5.2) |
| 8 | `models` | provider/model | normal tenant table | UK → `(tenant_id, provider_id, model_id)` |
| 9 | `model_variants` | provider/model | normal tenant table | |
| 10 | `search_providers` | provider/model | normal tenant table | |
| 11 | `fetch_providers` | provider/model | normal tenant table | |
| 12 | `memory_providers` | provider/model | normal tenant table | |
| 13 | `storage_providers` | provider/model | normal tenant table | |
| 14 | `provider_oauth_tokens` | provider/model | normal tenant table | |
| 15 | `user_provider_oauth_tokens` | provider/model | normal tenant table | |
| 16 | `bots` | bot | normal tenant table | `UNIQUE (tenant_id, name)` |
| 17 | `bot_acl_rules` | bot | normal tenant table | `UNIQUE NULLS NOT DISTINCT` prefixes `tenant_id` |
| 18 | `bot_channel_admins` | bot | normal tenant table | |
| 19 | `bot_user_grants` | bot | normal tenant table | |
| 20 | `bot_plugin_installations` | bot | normal tenant table | |
| 21 | `bot_plugin_resources` | bot | normal tenant table | |
| 22 | `bot_workspace_resource_limits` | bot | normal tenant table | PK → `(tenant_id, bot_id)` |
| 23 | `bot_channel_configs` | channel config | normal tenant table | |
| 24 | `bot_channel_routes` | channel config | normal tenant table | self/circular FK to `bot_sessions` |
| 25 | `mcp_connections` | MCP | normal tenant table | |
| 26 | `mcp_oauth_tokens` | MCP | normal tenant table | |
| 27 | `bot_sessions` | session/message | normal tenant table | self-ref `parent_session_id` composite |
| 28 | `bot_session_events` | session/message | normal tenant table | |
| 29 | `bot_history_messages` | session/message | normal tenant table | view `bot_visible_history_messages` must sync tenant_id |
| 30 | `bot_session_discuss_cursors` | session/message | normal tenant table | PK prefix `tenant_id` (`(tenant_id, session_id, scope_key)`) |
| 31 | `bot_history_message_assets` | session/message | normal tenant table | |
| 32 | `bot_history_message_compacts` | session/message | normal tenant table | |
| 33 | `tool_approval_requests` | interaction | normal tenant table | |
| 34 | `user_input_requests` | interaction | normal tenant table | |
| 35 | `containers` | workspace | normal tenant table | `container_id` uniqueness → tenant-internal |
| 36 | `snapshots` | workspace | normal tenant table | |
| 37 | `container_versions` | workspace | normal tenant table | |
| 38 | `lifecycle_events` | workspace | normal tenant table | TEXT PK `id`; PK → `(tenant_id, id)` |
| 39 | `schedule` | scheduling | normal tenant table | |
| 40 | `schedule_logs` | scheduling | normal tenant table | |
| 41 | `bot_heartbeat_logs` | scheduling | normal tenant table | |
| 42 | `email_providers` | email | normal tenant table | |
| 43 | `email_oauth_tokens` | email | normal tenant table | |
| 44 | `bot_email_bindings` | email | normal tenant table | |
| 45 | `email_outbox` | email | normal tenant table | |
| 46 | `bot_storage_bindings` | storage binding | normal tenant table | |
| 47 | `memory_nodes` | memory wiki | normal tenant table | TEXT PK; PK → `(tenant_id, id)` (§9) |
| 48 | `memory_edges` | memory wiki | normal tenant table | `BIGSERIAL` id; isolation via `(tenant_id, id)` not sequence (§9) |

## View

| View | Classification | Note |
|---|---|---|
| `bot_visible_history_messages` | derived (not separately allowlisted, §5.4) | Projects `bot_id`, `session_id`, `sender_channel_identity_id`, `sender_account_user_id`, etc. from `bot_history_messages`; filters `turn_visible = true AND turn_id/turn_position/turn_message_seq IS NOT NULL`. **Does NOT currently project or filter a tenant column** (none exists yet). Per §5.4 it must project/enforce `tenant_id` once the base table is tenantized and must not become a read bypass of tenant scope. |

## `待确认` summary (this inventory)

| Table | Disposition |
|---|---|
| `channel_identity_bind_codes` | Reverse drift: created in canonical 0001, dropped by incremental 0080. Confirm keep-vs-drop; default lean = treat as dropped, do not tenantize. |

Contract §14 also lists these historical/peripheral objects as `待确认`, but they are
**NOT** in the canonical 48 (they were created and/or dropped only in incrementals):
`media_assets`, `tasks`, `browser_contexts`, `bot_inbox`, `tts_providers`, `tts_models`,
`subagents`, `bot_members`, `bot_preauth_keys`, `email_provider_owner_map`, plus external
vector stores (Qdrant). Their final dispositions are analyzed in
`canonical_vs_incremental_diff.md`. Peripheral vector stores (Qdrant etc.) are an RFC
open question, out of scope for the relational canonical inventory.

## RESOLVED disposition (project decision)

The source of truth for the final schema is the **applied state** of the migration
chain on a real PostgreSQL 18 (`0001 → 0105`), NOT the text of `0001_init.up.sql`.
Existing migrations are frozen and never edited (see `BASE.md` migration policy).
The drift `待确认` items are therefore resolved by *what actually exists after full
apply*:

| Table | Applied-state fact | Resolved disposition |
|---|---|---|
| `media_assets` | created `0007`, never dropped → **EXISTS** | **normal tenant table** — tenantize in `0106+` |
| `tasks` | created `0063`, never dropped → **EXISTS** (VARCHAR PK, soft `bot_id`, no FK) | **normal tenant table** — tenantize in `0106+`; add composite PK `(tenant_id, id)` |
| `channel_identity_bind_codes` | in `0001`, **dropped by `0080`** → **DOES NOT EXIST** | **out of scope** — do not tenantize (already gone) |
| `browser_contexts`, `tts_providers`, `tts_models`, `bot_inbox`, `subagents`, `bot_members`, `bot_preauth_keys`, `email_provider_owner_map` | created & dropped in history → **DO NOT EXIST** | out of scope (already gone) |

Net: tenantization targets the **49-table applied final state** = canonical 48
− `channel_identity_bind_codes` + `media_assets` + `tasks`. Downstream tasks must
enumerate tenant tables by querying the applied schema (e.g. `information_schema`),
not by reading `0001_init.up.sql`.
