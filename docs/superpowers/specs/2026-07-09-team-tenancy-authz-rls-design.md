# Team Tenancy: Authorization + RLS Enforcement — Design

- **Date:** 2026-07-09
- **Branch:** `codex/team-multitenancy`
- **Status:** Proposed (design approved via brainstorming; pending written-spec review)

## 1. Problem

The team-multitenancy foundation added a `team_id` column to every tenant table
and team-scoped predicates to most sqlc queries, but the isolation is not
actually enforced end to end:

1. **Team resolution is a stub.** `teams.DefaultMiddleware()` (server.go:37) is
   the only team middleware wired, and it unconditionally injects the *default*
   team into every request. There is no per-user team resolution and no check
   that the caller belongs to the team. `teams.Scope` carries only `TeamID` — no
   user, no role.
2. **Authorization is global, not team-scoped.** `users.role` (`user_role` enum
   `member|admin`) is a single global flag. `accountService.IsAdmin(...)` reads
   it and gates ~15 handler sites. Per-team role (`team_members.role`,
   `owner|admin|member`) exists but nothing reads it for authz.
3. **App-layer isolation is by-convention and incomplete.** 35/43 query files
   carry `team_id` predicates, but some by-id lookups do not (e.g.
   `GetToolApprovalRequest` = `WHERE id = $1`), so a row is readable across teams
   by id. Gaps were found and fixed repeatedly this cycle; correctness depends on
   every query being wired correctly by hand.
4. **RLS is inert.** Policies exist (`ENABLE ROW LEVEL SECURITY` +
   `team_isolation USING team_id = current_setting('app.team_id')`) but RLS is
   **not** `FORCE`d and the app connects as role `memoh`, which **owns the tables
   and is a superuser** — both bypass non-forced RLS. Empirically proven: with a
   non-matching `app.team_id` set, the app role still reads all rows; a plain
   non-owner role correctly returns zero. So there is currently **no** database
   backstop.

## 2. Goals / Non-Goals

**Goals**

- Resolve the acting team per request and reject callers who are not members of
  it (tenant-level membership gate).
- Make `team_members.role` the single source of truth for authority; remove the
  global `users.role`.
- Make RLS actually enforce as a database-level backstop, by default, in every
  deployment.
- Keep the mechanism multi-team-capable so the SaaS product (an upstream
  consumer of open-source Memoh) can layer real multi-team on top, while the
  open-source build surfaces exactly one team.

**Non-Goals**

- A user-facing multi-team switcher / team management UI in open-source (SaaS
  owns that).
- Replacing the existing resource-level authz (bot ACL rules, `bot_user_grants`,
  channelaccess Manage). Those stay and operate *within* a team.
- A cross-team "platform super-admin" concept in open-source (SaaS can add its
  own if ever needed; it must not live on the open-source `users` table).

## 3. Locked Design Decisions

1. **Full three layers:** team resolution + membership/role authz + RLS
   enforcement.
2. **User↔team:** the data model is many-to-many (`team_members`). Open-source
   surfaces a single team; SaaS is the upstream layer that surfaces many. Team
   resolution is a **pluggable injection point** — open-source wires it to the
   single team, SaaS overrides it.
3. **Authz = membership gate.** Verify the authenticated user is a member of the
   resolved team; non-members are rejected. Resource-level ACL/grants are
   unchanged and complementary.
4. **`team_members.role` is the single source of truth for authority.**
   `users.role` + the `user_role` enum are removed. `users` stays a **global
   identity** table (no `team_id`; not team-owned). A team owns *membership* and
   *resources*, not *identity*. `IsAdmin` is repointed to the caller's role in
   the resolved team. Bootstrap makes the first user the team `owner`.
5. **RLS enforced by default everywhere (Option A).** Runtime connects as a
   dedicated non-owner, non-superuser role; migrations run as the owning/DDL
   role; all tenant tables get `FORCE ROW LEVEL SECURITY`; `app.team_id` is set
   per pooled connection from the request scope.

## 4. Architecture

Four cooperating layers. Each is independently testable.

### 4.1 Team resolution + membership gate (request edge)

A new middleware replaces the hardcoded `teams.DefaultMiddleware()` on
authenticated routes.

- **Runs after `auth.JWTMiddleware`** (server.go:64), because it needs the
  authenticated `user_id` (`auth.UserIDFromContext`). It shares the same public-
  route skipper as auth (login, health, inbound webhooks are unauthenticated and
  have no user — see 4.5).
- Uses a pluggable interface:

  ```go
  // TeamResolver resolves and authorizes the acting team for a request.
  type TeamResolver interface {
      // Resolve returns the team the user is acting in, or an error if the
      // user is not a member / not authorized.
      Resolve(ctx context.Context, userID string) (teams.Scope, error)
  }
  ```

- **Open-source implementation (`SingleTeamResolver`):** resolves to the single
  team (the default team) and verifies `team_members` contains `(team, user)`;
  if not, returns a not-a-member error → `403`. Loads the member's `role` into
  the scope.
- **SaaS** provides its own resolver (current team from JWT claim / request,
  membership + permission check). Not in this repo.
- `teams.Scope` gains the acting user and role so downstream authz reads them
  without re-querying:

  ```go
  type Scope struct {
      TeamID string
      UserID string       // acting user (empty for system/background contexts)
      Role   string       // team_members.role in this team: owner|admin|member
  }
  ```

- The resolved scope is injected into the request context (as today via
  `teams.WithScope`) and, for RLS, drives the per-connection `app.team_id`
  (4.4).

### 4.2 Authorization consolidation (`users.role` → `team_members.role`)

- **Drop** `users.role` and the `user_role` enum (migration, 5.1).
- **`accountService.IsAdmin`** changes from "read `users.role`" to "the resolved
  scope's role is `owner` or `admin`." Signature moves from
  `IsAdmin(ctx, channelIdentityID)` to reading `teams.ScopeFromContext(ctx)`
  (the ~15 call sites already run inside request context that now carries the
  scope). A thin helper:

  ```go
  func (s *Service) IsAdmin(ctx context.Context) (bool, error) // role in {owner,admin}
  ```

  Call sites in `internal/handlers/{users,session,message,acp_runtime,...}.go`
  switch to the ctx form. This is the largest mechanical surface of the change.
- **Bootstrap (`ensureAdminUser`)**: the first user is enrolled into the default
  team as `owner` instead of `users.role='admin'`. `teams.EnsureDefault` already
  enrolls members; extend it to grant `owner` to the bootstrap admin.
- **People page** (`internal/handlers/users.go`) semantics: "add person" ensures
  a global `users` row and creates a `team_members` link to the current team with
  a role; "remove person" deletes the `team_members` link (open-source single
  team ⇒ effectively removes them). Gated by the current team's `owner|admin`.
  A global `users` row with no memberships is inert (cannot resolve a team ⇒
  cannot pass the gate).

### 4.3 By-id query hardening (defense in depth with RLS)

- Audit every query for a missing `team_id` predicate (the `GetToolApprovalRequest`
  class). With RLS enforcing (4.4) these are *already* backstopped, but explicit
  predicates are kept so the app layer is correct on its own and errors are
  legible (not-found vs forbidden). Output: a checklist of by-id queries to add
  `AND team_id = sqlc.arg(team_id)` to.

### 4.4 RLS enforcement (database backstop)

- **Migration (DDL role):**
  - `ALTER TABLE ... FORCE ROW LEVEL SECURITY` on all tenant tables (the
    `team_isolation` policy already exists).
  - Create role `memoh_app`: `LOGIN`, **not** superuser, **not** owner; `GRANT
    SELECT, INSERT, UPDATE, DELETE` on all tenant tables + `teams`,
    `team_members`, `users`; `GRANT USAGE, SELECT` on sequences; `GRANT USAGE` on
    schema `public`. Ownership stays with the DDL role.
- **Runtime connection:** the app connects as `memoh_app`. Migrations continue to
  run as the owning/DDL role.
- **Per-connection scope injection:** in the pgx pool construction
  (`internal/db/db.go`), add `AfterConnect`/`BeforeAcquire` so each checked-out
  connection runs `SELECT set_config('app.team_id', $1, false)` from the request
  scope's `TeamID`. `team_dbtx`'s existing `SET LOCAL` inside real transactions
  is retained; the singleton path (refactored earlier to run directly on the
  pool) gets its scope from the pool hook.
- **Process-wide / background contexts** (startup reconcile, bootstrap, the
  all-team infrastructure queries) have no per-request team and legitimately span
  teams. Under `FORCE` RLS this is a trap: the policy is `team_id = NULLIF(
  current_setting('app.team_id', true), '')::uuid`, so on the restricted role
  with `app.team_id` **unset**, `team_id = NULL` is never true and every
  all-team query returns **zero rows**. Therefore these paths must run on a
  connection that **bypasses RLS** — a dedicated maintenance role with
  `BYPASSRLS` (or the owning/DDL role) — used only for the four all-team infra
  queries (`ListAutoStartContainers`, `ListEnabledSchedules`,
  `ListHeartbeatEnabledBots`, `ListBotChannelConfigsByType`) and startup
  bootstrap. This is a small, enumerated allowlist held on a separate pool, not
  the general query path. Everything downstream then re-scopes per row's
  `team_id` on the restricted pool.

### 4.5 Unauthenticated surfaces

- **Login, health, static:** no team, no scope; not gated.
- **Inbound channel webhooks:** arrive without a user. They resolve the target
  bot/config across teams (the four infra queries are already all-team) and then
  operate under that resource's `team_id`. The webhook path sets `app.team_id`
  from the resolved resource row, not from a user scope.

## 5. Data Model Changes

New migration `0106_team_authz_rls`:

- `ALTER TABLE users ALTER COLUMN role ...` → **drop** `users.role`; `DROP TYPE
  user_role` (after dependent objects removed). Down: recreate enum + column,
  backfill from `team_members` (`owner|admin` → `admin`, else `member`).
- `ALTER TABLE <tenant> FORCE ROW LEVEL SECURITY` for every tenant table (mirror
  the ENABLE list in 0105).
- `CREATE ROLE memoh_app ...` + GRANTs (idempotent guards).
- `0001_init.up.sql` updated to the final state (drop `users.role`/enum, FORCE
  RLS, role+grants) per the repo's canonical-schema rule.
- No new columns. `users` stays global (no `team_id`).

## 6. Deployment / Config Changes

- **Config:** `PostgresConfig` gains a runtime credential (`memoh_app`) distinct
  from the migration credential (owner). Two runtime pools: the **main restricted
  pool** (`memoh_app`, RLS-enforced) for all request/per-team work, and a small
  **maintenance pool** on the owner/`BYPASSRLS` DSN used only for the four
  enumerated all-team startup queries (4.4). Migrations use the owner DSN.
- **docker-compose / entrypoints:** create `memoh_app` (via migration) and pass
  its credentials to the `server` service; the `migrate` service keeps the owner
  credentials.
- **Example configs** (`conf/app.example.toml`, docker/apple/windows) updated
  with the runtime role.
- Because Option A makes this the default, the open-source `docker compose up`
  path must provision the role automatically (migration-created role + compose
  env), keeping self-host one-command.

## 7. Error Handling

- Not a member of the resolved team → `403 Forbidden` (distinct from `401`
  unauthenticated).
- Role insufficient for an admin action → `403`.
- Scope missing where required (a bug: authenticated route without resolution) →
  `500`, logged; never silently fall back to the default team on authenticated
  routes.
- RLS-filtered by-id lookups return not-found, not another team's row.

## 8. Testing Strategy

- **Resolver/middleware:** unit tests — member passes with role in scope;
  non-member gets 403; public routes skip.
- **Authz:** `IsAdmin` reads scope role; owner/admin pass, member fails; the
  ~15 call sites covered by handler tests where they exist.
- **RLS (integration, real Postgres):** as `memoh_app` with `app.team_id` = team
  A, cannot read/update/delete team B rows (INSERT/SELECT/UPDATE/DELETE); with
  the owner role, migrations still work. Extends the existing
  `team_dbtx`/contract tests. This is the test that proves "形同虚设" is fixed.
- **By-id hardening:** contract test asserting the audited by-id queries carry
  `team_id`.
- **Migration:** up/down/up cycle on a scratch DB (as in CI); verify `users.role`
  gone, enum gone, FORCE set, role present; down restores.
- Full `go build` / `go vet` / `go test` / `golangci-lint` / sqlc idempotency.

## 9. Phasing (sequenced; each phase independently shippable)

1. **Scope carries user+role; resolver + membership-gate middleware** (open-source
   single-team resolver). Wire after auth; keep default behavior for public
   routes.
2. **Authz consolidation:** drop `users.role`/enum (migration 0106 part 1),
   repoint `IsAdmin` + ~15 call sites, bootstrap owner enrollment.
3. **RLS enforcement:** FORCE + `memoh_app` role + grants (migration 0106 part 2),
   pool `app.team_id` hook, config/compose runtime role, background-context
   handling.
4. **By-id query hardening** + contract test.

Phases 1–2 are app-layer and low deployment risk. Phase 3 carries the deployment
change (role/connection) and needs the integration RLS tests before merge.

## 10. Risks / Open Questions

- **`IsAdmin` blast radius (~15 sites).** Mechanical but wide; a couple take a
  `channelIdentityID` and resolve the user differently — verify each maps to the
  request scope cleanly.
- **Background/webhook `app.team_id`.** Under FORCE RLS a restricted connection
  with no `app.team_id` returns zero rows, so startup reconcile/bootstrap must
  use a `BYPASSRLS` (or owner) maintenance pool for the four enumerated all-team
  queries (4.4). Webhook routing derives the team from the resolved resource row
  and then sets `app.team_id` for subsequent restricted-pool work (4.5). Confirm
  no enforced-RLS query sits on a scopeless restricted connection.
- **Managed Postgres.** Creating roles via migration assumes sufficient
  privilege; document the owner/DDL credential requirement for hosted DBs.
- **Open-source value of RLS at one team is ~nil** (there is no second tenant to
  block); Option A accepts the added self-host complexity so the mechanism is
  uniform and SaaS inherits real isolation for free. Re-confirm this trade if
  self-host friction becomes a support burden.
